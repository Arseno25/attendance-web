import hmac
import os
import threading
from uuid import UUID, uuid4

import cv2
import onnxruntime as ort
from cryptography.exceptions import InvalidTag
from fastapi import Depends, FastAPI, Request
from fastapi.exceptions import RequestValidationError
from fastapi.responses import JSONResponse
from pydantic import BaseModel, ConfigDict, Field
from starlette.exceptions import HTTPException

from app.pipeline import ImageError, get_cipher, get_pipeline, status
from app.recognition import get_recognition_service

app = FastAPI(
    title="Attendance AI (internal)", docs_url=None, redoc_url=None, openapi_url=None
)
_preview_slot = threading.BoundedSemaphore(1)
_recognition_slot = threading.BoundedSemaphore(1)


def error(request: Request, status_code: int, code: str, message: str):
    return JSONResponse(
        status_code=status_code,
        headers={
            "X-Request-ID": request.state.request_id,
            "Cache-Control": "no-store",
            "X-Content-Type-Options": "nosniff",
        },
        content={
            "error": {"code": code, "message": message},
            "request_id": request.state.request_id,
        },
    )


def require_service(request: Request):
    expected = os.getenv("AI_SERVICE_TOKEN", "")
    supplied = request.headers.get("authorization", "")
    if len(expected) < 32 or not hmac.compare_digest(
        supplied.encode(), ("Bearer " + expected).encode()
    ):
        raise HTTPException(status_code=401)


@app.middleware("http")
async def request_metadata(request: Request, call_next):
    request.state.request_id = str(uuid4())
    if request.url.path not in (
        "/internal/v1/health/live",
        "/internal/v1/health/ready",
    ):
        try:
            require_service(request)
        except HTTPException:
            return error(request, 401, "UNAUTHENTICATED", "Request tidak diizinkan")
    if request.method == "POST":
        limit = {
            "/internal/v1/enroll": 6 << 20,
            "/internal/v1/face-preview": 512 << 10,
        }.get(request.url.path, 6 << 20)
        total, chunks = 0, []
        async for chunk in request.stream():
            total += len(chunk)
            if total > limit:
                return error(request, 413, "PAYLOAD_TOO_LARGE", "Request terlalu besar")
            chunks.append(chunk)
        request._body = b"".join(chunks)
    response = await call_next(request)
    response.headers["X-Request-ID"] = request.state.request_id
    response.headers["Cache-Control"] = "no-store"
    response.headers["X-Content-Type-Options"] = "nosniff"
    return response


@app.exception_handler(HTTPException)
async def http_error(request: Request, exc: HTTPException):
    code = {401: "UNAUTHENTICATED", 404: "NOT_FOUND", 405: "METHOD_NOT_ALLOWED"}.get(
        exc.status_code, "HTTP_ERROR"
    )
    response = error(request, exc.status_code, code, "Request tidak diizinkan")
    response.headers.update(exc.headers or {})
    return response


@app.exception_handler(RequestValidationError)
async def validation_error(request: Request, exc: RequestValidationError):
    return error(request, 422, "VALIDATION_ERROR", "Request tidak valid")


@app.exception_handler(ImageError)
async def image_error(request: Request, exc: ImageError):
    return error(request, 422, exc.code, exc.message)


class StrictBody(BaseModel):
    model_config = ConfigDict(extra="forbid")


class FacePreview(StrictBody):
    image: str = Field(min_length=1, max_length=524288, repr=False)


class GalleryTemplate(StrictBody):
    student_id: UUID
    consent_id: UUID | None = None
    ciphertext: str = Field(max_length=704, repr=False)
    nonce: str = Field(max_length=16, repr=False)
    model_sha256: str = Field(pattern=r"^[a-f0-9]{64}$")
    key_id: str = Field(min_length=1, max_length=80)


class Enrollment(StrictBody):
    school_id: UUID
    student_id: UUID
    consent_id: UUID | None = None
    frames: list[str] = Field(min_length=3, max_length=5, repr=False)
    gallery: list[GalleryTemplate] = Field(
        default_factory=list, max_length=3000, repr=False
    )


class Recognition(StrictBody):
    school_id: UUID
    session_id: UUID
    device_id: UUID
    event_id: UUID
    track_id: UUID
    image: str = Field(max_length=524288, repr=False)
    gallery_revision: int = Field(ge=0)
    gallery: list[GalleryTemplate] = Field(max_length=3000, repr=False)


@app.get("/internal/v1/health/live")
def live(request: Request):
    if "CPUExecutionProvider" not in ort.get_available_providers():
        return error(request, 503, "RUNTIME_UNAVAILABLE", "Runtime belum siap")
    return {"status": "ok", "request_id": request.state.request_id}


@app.get("/internal/v1/health/ready")
def ready(request: Request):
    result = status()
    if not result["pipeline_ready"]:
        return error(
            request,
            503,
            "PIPELINE_NOT_CONFIGURED",
            "Model atau kunci enkripsi belum tersedia",
        )
    return dict(result, status="ok", request_id=request.state.request_id)


@app.get("/internal/v1/status", dependencies=[Depends(require_service)])
def pipeline_status():
    return status()


@app.post("/internal/v1/enroll", dependencies=[Depends(require_service)])
def enroll(body: Enrollment, request: Request):
    try:
        pipeline, cipher = get_pipeline(), get_cipher()
    except OSError, ValueError, cv2.error:
        return error(
            request,
            503,
            "PIPELINE_NOT_CONFIGURED",
            "Model atau kunci enkripsi belum tersedia",
        )
    try:
        result = pipeline.enroll(
            body.frames,
            cipher,
            str(body.school_id),
            str(body.student_id),
            str(body.consent_id) if body.consent_id is not None else None,
            gallery=[entry.model_dump(mode="json") for entry in body.gallery],
            duplicate_threshold=float(os.getenv("AI_DUPLICATE_THRESHOLD", "0.85")),
        )
    except (InvalidTag, ValueError) as exc:
        if isinstance(exc, ImageError):
            raise
        return error(
            request,
            503,
            "GALLERY_UNAVAILABLE",
            "Data wajah belum dapat diperiksa. Hubungi pengelola.",
        )
    return dict(
        result, recognition_enabled=False, block_reason="MODEL_LIVENESS_NOT_EVALUATED"
    )


@app.post("/internal/v1/recognize", dependencies=[Depends(require_service)])
def recognize(body: Recognition, request: Request):
    if not _recognition_slot.acquire(blocking=False):
        return error(
            request, 503, "INFERENCE_BUSY", "Kamera sedang diproses; coba lagi"
        )
    try:
        service = get_recognition_service()
        result = service.process(
            body.image,
            school_id=str(body.school_id),
            session_id=str(body.session_id),
            device_id=str(body.device_id),
            track_id=str(body.track_id),
            gallery=[entry.model_dump(mode="json") for entry in body.gallery],
        )
        accepted = (
            bool(service.evaluation)
            and result.status == "CONFIRMED"
            and result.liveness_passed is True
        )
        faces = [dict(face) for face in result.faces]
        if accepted:
            faces[0]["student_id"] = result.student_id
        return {
            "decision": "accepted"
            if accepted
            else "no_face"
            if result.status == "NO_FACE"
            else "inconclusive"
            if result.status in ("MULTIPLE_FACES", "LOW_QUALITY", "ERROR")
            else "unknown",
            "faces": faces,
            "engine_status": result.status,
            "recognition_enabled": accepted,
            "block_reason": ""
            if service.evaluation
            else "MODEL_LIVENESS_NOT_EVALUATED",
            "evaluation": service.evaluation or {},
            "event_id": str(body.event_id),
            "gallery_revision": body.gallery_revision,
            "model_sha256": service.pipeline.model_sha256,
        }
    except ImageError:
        raise
    except OSError, ValueError, InvalidTag, cv2.error:
        return error(
            request,
            503,
            "PIPELINE_NOT_CONFIGURED",
            "Layanan wajah belum siap. Hubungi pengelola.",
        )
    finally:
        _recognition_slot.release()


@app.post("/internal/v1/face-preview", dependencies=[Depends(require_service)])
def face_preview(body: FacePreview, request: Request):
    if not _preview_slot.acquire(blocking=False):
        return error(
            request, 503, "INFERENCE_BUSY", "Kamera sedang diproses; coba lagi"
        )
    try:
        return get_pipeline().preview(body.image)
    except ImageError:
        raise
    except OSError, ValueError, cv2.error:
        return error(
            request, 503, "PIPELINE_NOT_CONFIGURED", "Model kamera belum tersedia"
        )
    finally:
        _preview_slot.release()
