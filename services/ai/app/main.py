import hmac
import os
from uuid import uuid4

import onnxruntime as ort
from fastapi import Depends, FastAPI, Request
from fastapi.exceptions import RequestValidationError
from fastapi.responses import JSONResponse
from starlette.exceptions import HTTPException

app = FastAPI(
    title="Attendance AI (internal)", docs_url=None, redoc_url=None, openapi_url=None
)


@app.middleware("http")
async def request_metadata(request: Request, call_next):
    request.state.request_id = str(uuid4())
    response = await call_next(request)
    response.headers["X-Request-ID"] = request.state.request_id
    response.headers["Cache-Control"] = "no-store"
    response.headers["X-Content-Type-Options"] = "nosniff"
    return response


def error(request: Request, status: int, code: str, message: str):
    return JSONResponse(
        status_code=status,
        content={
            "error": {"code": code, "message": message},
            "request_id": request.state.request_id,
        },
    )


@app.exception_handler(HTTPException)
async def http_error(request: Request, exc: HTTPException):
    code = {
        401: "UNAUTHENTICATED",
        404: "NOT_FOUND",
        405: "METHOD_NOT_ALLOWED",
    }.get(exc.status_code, "HTTP_ERROR")
    response = error(request, exc.status_code, code, "Request tidak diizinkan")
    response.headers.update(exc.headers or {})
    return response


@app.exception_handler(RequestValidationError)
async def validation_error(request: Request, exc: RequestValidationError):
    return error(request, 422, "VALIDATION_ERROR", "Request tidak valid")


def require_service(request: Request):
    expected = os.getenv("AI_SERVICE_TOKEN", "")
    supplied = request.headers.get("authorization", "")
    if len(expected) < 32 or not hmac.compare_digest(
        supplied.encode(), ("Bearer " + expected).encode()
    ):
        raise HTTPException(status_code=401)


@app.get("/internal/v1/health/live")
def live(request: Request):
    if "CPUExecutionProvider" not in ort.get_available_providers():
        return error(request, 503, "RUNTIME_UNAVAILABLE", "Runtime belum siap")
    return {"status": "ok", "request_id": request.state.request_id}


@app.get("/internal/v1/health/ready")
def ready(request: Request):
    return error(request, 503, "MODEL_NOT_CONFIGURED", "Model belum dikonfigurasi")


@app.post("/internal/v1/recognize", dependencies=[Depends(require_service)])
def recognize(request: Request):
    # shortcut: inference is disabled until model, liveness and calibration pass WP-06.
    return error(request, 503, "RECOGNITION_DISABLED", "Pengenalan belum diaktifkan")
