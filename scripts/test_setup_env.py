import shutil
import stat
import subprocess
import tempfile
import unittest
from pathlib import Path


class SetupEnvTest(unittest.TestCase):
    def test_creates_private_secrets_and_preserves_existing_configuration(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "scripts").mkdir()
            script = root / "scripts/setup_env.py"
            shutil.copyfile(Path(__file__).with_name("setup_env.py"), script)
            result = subprocess.run(["python3", str(script)], capture_output=True, text=True, check=True)
            env = root / ".env"
            content = env.read_text()
            values = dict(line.split("=", 1) for line in content.splitlines())
            self.assertGreaterEqual(len(values["AI_SERVICE_TOKEN"]), 32)
            self.assertIn(values["POSTGRES_PASSWORD"], values["DATABASE_URL"])
            self.assertNotIn(values["POSTGRES_PASSWORD"], result.stdout)
            self.assertEqual(stat.S_IMODE(env.stat().st_mode), 0o600)
            subprocess.run(["python3", str(script)], capture_output=True, check=True)
            self.assertEqual(env.read_text(), content)


if __name__ == "__main__":
    unittest.main()
