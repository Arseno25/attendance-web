import unittest
from contextlib import redirect_stderr, redirect_stdout
from io import StringIO
from types import SimpleNamespace
from unittest.mock import patch

from scripts.compose_web import start_web


class ComposeWebLauncherTests(unittest.TestCase):
    def test_retries_next_port_only_when_host_port_is_in_use(self):
        results = iter(
            [
                SimpleNamespace(
                    returncode=1,
                    stdout="",
                    stderr="rootlessport listen tcp 127.0.0.1:3001: bind: address already in use",
                ),
                SimpleNamespace(returncode=0, stdout="started\n", stderr=""),
            ]
        )
        calls = []

        def run(command, *, cwd, env, capture_output, text):
            calls.append((command, env["WEB_HOST_PORT"], env.get("WEB_ORIGIN"), env.get("ALLOW_LOCAL_WEB_PORT_RANGE")))
            return next(results)

        with patch("builtins.print") as output:
            self.assertEqual(start_web(run=run), 0)

        self.assertEqual([port for _, port, _, _ in calls], ["3001", "3002"])
        self.assertEqual([origin for _, _, origin, _ in calls], ["http://localhost:3001", "http://localhost:3002"])
        self.assertEqual([local_origins for _, _, _, local_origins in calls], ["1", "1"])
        self.assertEqual(output.call_args_list[-1].args, ("Web tersedia di http://localhost:3002",))
        self.assertEqual(calls[1][0][-3:], ["up", "-d", "web"])

    def test_does_not_retry_unrelated_compose_failures(self):
        calls = []
        failure = SimpleNamespace(
            returncode=17, stdout="", stderr="invalid compose config"
        )

        def run(command, *, cwd, env, capture_output, text):
            calls.append(env["WEB_HOST_PORT"])
            return failure

        with redirect_stdout(StringIO()), redirect_stderr(StringIO()):
            self.assertEqual(start_web(run=run), 17)
        self.assertEqual(calls, ["3001"])


if __name__ == "__main__":
    unittest.main()
