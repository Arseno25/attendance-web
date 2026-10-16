import json
import os
import shutil
import subprocess
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]


class ComposeWebPortTests(unittest.TestCase):
    def test_coolify_project_directory_resolves_all_builds_inside_checkout(self):
        with tempfile.TemporaryDirectory() as directory:
            checkout = Path(directory) / "checkout"
            (checkout / "deploy").mkdir(parents=True)
            for name in ["compose.application.yml", ".env.example"]:
                shutil.copy2(ROOT / "deploy" / name, checkout / "deploy" / name)
            expected = {
                "api": "services/api",
                "migrate": "services/api",
                "ai": "services/ai",
                "init-models": "services/ai",
                "web": "apps/web",
            }
            for context in set(expected.values()):
                (checkout / context).mkdir(parents=True)
                shutil.copy2(
                    ROOT / context / "Dockerfile", checkout / context / "Dockerfile"
                )
            result = subprocess.run(
                [
                    "docker",
                    "compose",
                    "--env-file",
                    str(checkout / "deploy/.env.example"),
                    "--project-name",
                    "coolify-regression",
                    "--project-directory",
                    str(checkout),
                    "-f",
                    str(checkout / "deploy/compose.application.yml"),
                    "config",
                    "--format",
                    "json",
                ],
                cwd=directory,
                check=True,
                capture_output=True,
                text=True,
            )
            services = json.loads(result.stdout)["services"]
            for service, context in expected.items():
                with self.subTest(service=service):
                    build = services[service]["build"]
                    self.assertEqual(Path(build["context"]), checkout / context)
                    self.assertTrue(
                        (Path(build["context"]) / build["dockerfile"]).is_file()
                    )

    def test_web_uses_container_port_3000_and_selectable_loopback_host_port(self):
        result = subprocess.run(
            [
                "docker",
                "compose",
                "--project-directory",
                str(ROOT),
                "--env-file",
                "deploy/.env.example",
                "-f",
                "deploy/compose.application.yml",
                "config",
                "--format",
                "json",
            ],
            cwd=ROOT,
            check=True,
            capture_output=True,
            text=True,
        )
        services = json.loads(result.stdout)["services"]
        web = services["web"]
        api = services["api"]
        self.assertNotIn("SCHOOL_ID", api["environment"])
        self.assertEqual(api["environment"]["ALLOW_LOCAL_WEB_PORT_RANGE"], "0")
        self.assertEqual(web["environment"]["PORT"], "3000")
        self.assertEqual(web["expose"], ["3000"])
        self.assertEqual(len(web["ports"]), 1)
        port = web["ports"][0]
        self.assertEqual(port["target"], 3000)
        self.assertEqual(port["published"], "3001")
        self.assertEqual(port["host_ip"], "127.0.0.1")

        result = subprocess.run(
            [
                "docker",
                "compose",
                "--project-directory",
                str(ROOT),
                "--env-file",
                "deploy/.env.example",
                "-f",
                "deploy/compose.application.yml",
                "config",
                "--format",
                "json",
            ],
            cwd=ROOT,
            env={**os.environ, "WEB_HOST_PORT": "3002"},
            check=True,
            capture_output=True,
            text=True,
        )
        services = json.loads(result.stdout)["services"]
        web = services["web"]
        api = services["api"]
        self.assertNotIn("SCHOOL_ID", api["environment"])
        self.assertEqual(api["environment"]["ALLOW_LOCAL_WEB_PORT_RANGE"], "0")
        self.assertEqual(web["environment"]["PORT"], "3000")
        self.assertEqual(web["ports"][0]["published"], "3002")

    def test_deployment_models_are_persistent_and_migration_gates_api(self):
        result = subprocess.run(
            [
                "docker",
                "compose",
                "--project-directory",
                str(ROOT),
                "-p",
                "deployment-test",
                "--env-file",
                "deploy/.env.example",
                "-f",
                "deploy/compose.application.yml",
                "config",
                "--format",
                "json",
            ],
            cwd=ROOT,
            check=True,
            capture_output=True,
            text=True,
        )
        config = json.loads(result.stdout)
        services = config["services"]
        model_init = services["init-models"]
        model_runtime = services["ai"]
        migration = services["migrate"]
        api = services["api"]

        self.assertEqual(model_init["build"]["context"], str(ROOT / "services/ai"))
        self.assertEqual(
            model_init["command"],
            ["python", "-m", "app.models", "--directory", "/models"],
        )
        self.assertEqual(model_init["user"], "0:0")
        self.assertEqual(model_init["restart"], "no")
        self.assertNotIn("depends_on", model_init)
        init_mount = next(
            mount for mount in model_init["volumes"] if mount["target"] == "/models"
        )
        runtime_mount = next(
            mount for mount in model_runtime["volumes"] if mount["target"] == "/models"
        )
        self.assertEqual(init_mount["type"], "volume")
        self.assertEqual(
            config["volumes"]["model_data"]["name"], "deployment-test_model_data"
        )
        self.assertEqual(runtime_mount["type"], "volume")
        self.assertEqual(init_mount["source"], runtime_mount["source"])
        self.assertFalse(init_mount.get("read_only", False))
        self.assertTrue(runtime_mount["read_only"])
        self.assertEqual(
            model_runtime["depends_on"]["init-models"]["condition"],
            "service_completed_successfully",
        )

        self.assertEqual(migration["build"]["context"], str(ROOT / "services/api"))
        self.assertEqual(migration["restart"], "no")
        self.assertEqual(migration["environment"]["GOOSE_DRIVER"], "postgres")
        env_values = dict(
            line.split("=", 1)
            for line in (ROOT / "deploy/.env.example").read_text().splitlines()
            if "=" in line and not line.startswith("#")
        )
        self.assertEqual(
            migration["environment"]["GOOSE_DBSTRING"],
            env_values["MIGRATION_DATABASE_URL"],
        )
        self.assertNotEqual(
            migration["environment"]["GOOSE_DBSTRING"],
            api["environment"]["DATABASE_URL"],
        )
        self.assertNotIn("GOOSE_DBSTRING", api["environment"])
        self.assertNotIn("DATABASE_URL", migration["environment"])
        self.assertEqual(
            migration["command"], ["goose", "-dir", "/app/migrations", "up"]
        )
        self.assertEqual(
            api["depends_on"]["migrate"]["condition"], "service_completed_successfully"
        )

        compose_source = (ROOT / "deploy/compose.application.yml").read_text()
        self.assertIn("${MIGRATION_DATABASE_URL:?", compose_source)
        config_env = {
            **os.environ,
            **{
                key: value
                for key, value in env_values.items()
                if key != "MIGRATION_DATABASE_URL"
            },
        }
        config_env.pop("MIGRATION_DATABASE_URL", None)
        missing_migration_url = subprocess.run(
            [
                "docker",
                "compose",
                "--project-directory",
                str(ROOT),
                "--env-file",
                "/dev/null",
                "-f",
                "deploy/compose.application.yml",
                "config",
                "--quiet",
            ],
            cwd=ROOT,
            env=config_env,
            capture_output=True,
            text=True,
            check=False,
        )
        self.assertNotEqual(missing_migration_url.returncode, 0)
        self.assertIn("MIGRATION_DATABASE_URL", missing_migration_url.stderr)
        self.assertNotIn("AI_MODELS_HOST_PATH", result.stdout)


if __name__ == "__main__":
    unittest.main()
