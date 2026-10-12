"""Create ignored local development secrets without overwriting an existing .env."""

import secrets
from pathlib import Path


def main():
    path = Path(__file__).resolve().parents[1] / ".env"
    password = secrets.token_hex(24)
    content = (
        "POSTGRES_DB=attendance\nPOSTGRES_USER=attendance\n"
        "SCHOOL_ID=00000000-0000-4000-8000-000000000001\n"
        f"POSTGRES_PASSWORD={password}\n"
        f"DATABASE_URL=postgresql://attendance:{password}@127.0.0.1:54329/attendance?sslmode=disable\n"
        "API_ADDRESS=127.0.0.1:8080\nWEB_ORIGIN=http://localhost:3000\n"
        "NUXT_PUBLIC_API_BASE=http://localhost:8080/api/v1\n"
        "NUXT_PORT=3000\n"
        f"AI_SERVICE_TOKEN={secrets.token_hex(32)}\n"
    )
    try:
        with path.open("x") as file:
            path.chmod(0o600)
            file.write(content)
    except FileExistsError:
        print("Existing .env preserved.")
    else:
        print("Local .env created (secrets omitted).")


if __name__ == "__main__":
    main()
