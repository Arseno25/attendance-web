import pytest
from fastapi.testclient import TestClient

from app.main import app


def test_liveness_and_readiness_do_not_claim_a_missing_model_is_ready(monkeypatch):
    monkeypatch.setenv("AI_MODELS_DIR", "/missing-models")
    from app.pipeline import get_pipeline

    get_pipeline.cache_clear()
    with TestClient(app) as client:
        assert client.get("/internal/v1/health/live").status_code == 200
        result = client.get("/internal/v1/health/ready")
        assert result.status_code == 503
        assert result.json()["error"]["code"] == "PIPELINE_NOT_CONFIGURED"
        assert result.json()["request_id"] == result.headers["x-request-id"]


@pytest.mark.parametrize("token", [None, "wrong-token"])
def test_internal_recognition_rejects_unauthorized_callers(monkeypatch, token):
    monkeypatch.setenv("AI_SERVICE_TOKEN", "a" * 48)
    headers = {"Authorization": f"Bearer {token}"} if token else {}
    with TestClient(app) as client:
        result = client.post("/internal/v1/recognize", headers=headers)
        assert result.status_code == 401
        assert "candidate_student_id" not in result.text


def test_authenticated_recognition_is_fail_closed_until_model_evaluated(monkeypatch):
    monkeypatch.setenv("AI_SERVICE_TOKEN", "a" * 48)
    with TestClient(app) as client:
        result = client.post(
            "/internal/v1/recognize", headers={"Authorization": "Bearer " + "a" * 48}
        )
        assert result.status_code == 422
        assert result.json()["error"]["code"] == "VALIDATION_ERROR"
        assert "candidate_student_id" not in result.text


def test_missing_service_secret_cannot_authorize_empty_bearer(monkeypatch):
    monkeypatch.delenv("AI_SERVICE_TOKEN", raising=False)
    with TestClient(app) as client:
        result = client.post(
            "/internal/v1/recognize", headers={"Authorization": "Bearer "}
        )
        assert result.status_code == 401


def test_wrong_method_returns_method_error_and_allow_header():
    with TestClient(app) as client:
        result = client.post("/internal/v1/health/live")
        assert result.status_code == 405
        assert result.json()["error"]["code"] == "METHOD_NOT_ALLOWED"
        assert "GET" in result.headers["allow"]
