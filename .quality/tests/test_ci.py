import importlib.util
import json
import sys
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

QUALITY = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(QUALITY))
spec = importlib.util.spec_from_file_location("quality_ci", QUALITY / "ci.py")
if spec is None or spec.loader is None:
    raise ImportError("Unable to load the CI runner")
ci = importlib.util.module_from_spec(spec)
spec.loader.exec_module(ci)


class CIJobTests(unittest.TestCase):
    def run_job(self, job):
        config = json.loads((QUALITY / "ci.json").read_text())
        executed = []

        def execute(entry, timeout):
            executed.append(entry)
            ci.record(entry["name"], "PASSED")
            return True

        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / ".quality").mkdir()
            (root / ".quality/ci.json").write_text(json.dumps(config))
            with patch.object(ci, "ROOT", root), patch.object(ci, "execute", execute):
                self.assertEqual(ci.main(["job", job]), 0)
                self.assertEqual((root / ".tools/ci/job-status.txt").read_text().strip(), "PASSED")
        return executed

    def test_browser_installs_chromium_before_browser_tests(self):
        entries = self.run_job("browser")
        commands = [entry["argv"] for entry in entries]
        install = ["npm", "run", "playwright:install"]
        browser = ["npm", "run", "test", "--", "tests/landing.test.ts"]
        self.assertLess(commands.index(install), commands.index(browser))
        self.assertNotIn(["npm", "run", "test", "--", "tests/health.test.ts"], commands)

    def test_general_runs_http_tests_without_browser_setup(self):
        commands = [entry["argv"] for entry in self.run_job("general")]
        self.assertIn(["npm", "run", "test", "--", "tests/health.test.ts"], commands)
        self.assertNotIn(["npm", "run", "playwright:install"], commands)
        self.assertNotIn(["npm", "run", "test", "--", "tests/landing.test.ts"], commands)

    def test_types_does_not_install_chromium(self):
        commands = [entry["argv"] for entry in self.run_job("types")]
        self.assertNotIn(["npm", "run", "playwright:install"], commands)

    def test_invalid_setup_suite_is_rejected(self):
        config = json.loads((QUALITY / "ci.json").read_text())
        config["setup"][-1]["suite"] = "invalid"
        with self.assertRaises(ValueError):
            ci.validate_config(config)


if __name__ == "__main__":
    unittest.main()
