import os
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
COMPOSE = [
    "docker",
    "compose",
    "--project-directory",
    str(ROOT),
    "-p",
    "attendance",
    "--env-file",
    ".env",
    "-f",
    "deploy/compose.application.yml",
    "up",
    "-d",
    "web",
]
PORT_CONFLICTS = (
    "address already in use",
    "address already allocated",
    "port is already allocated",
    "cannot listen on port",
    "rootlessport listen tcp",
    "failed to bind host port",
)


def start_web(run=subprocess.run, start_port=3001, end_port=3010):
    for port in range(start_port, end_port + 1):
        env = os.environ.copy()
        env["WEB_HOST_PORT"] = str(port)
        env["WEB_ORIGIN"] = f"http://localhost:{port}"
        env["ALLOW_LOCAL_WEB_PORT_RANGE"] = "1"
        result = run(COMPOSE, cwd=ROOT, env=env, capture_output=True, text=True)
        if result.returncode == 0:
            sys.stdout.write(result.stdout)
            sys.stderr.write(result.stderr)
            print(f"Web tersedia di http://localhost:{port}")
            return 0

        output = f"{result.stdout}\n{result.stderr}"
        if not any(message in output.lower() for message in PORT_CONFLICTS):
            sys.stdout.write(result.stdout)
            sys.stderr.write(result.stderr)
            return result.returncode
        print(
            f"Port {port} sedang digunakan; mencoba port berikutnya.", file=sys.stderr
        )

    print(f"Port {start_port}-{end_port} semuanya sedang digunakan.", file=sys.stderr)
    return 1


if __name__ == "__main__":
    raise SystemExit(start_web())
