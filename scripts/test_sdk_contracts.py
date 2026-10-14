#!/usr/bin/env python3
"""Compile/import the manually maintained, offline GitHub SDK contract fixtures."""
import os
from pathlib import Path
import subprocess
import sys
import tempfile

ROOT = Path(__file__).resolve().parents[1]
FIXTURES = ROOT / "examples" / "sdk-contracts"
SAFE = Path("/tmp/opencode/gitlab-integration-tools-fwQ0lcJ2/bin/safe-run")


def run(language, command, **kwargs):
    print(f"[{language}] {' '.join(map(str, command))}", flush=True)
    subprocess.run([str(SAFE), *map(str, command)], cwd=FIXTURES / language, check=True, **kwargs)


def main():
    run("go", ["go", "test", "./..."])
    run("nodejs", ["npm", "install", "--package-lock=false", "--ignore-scripts", "--no-audit", "--no-fund"])
    run("nodejs", ["npx", "tsc", "--noEmit"])
    venv = Path(tempfile.gettempdir()) / "opencode" / "sdk-contract-python"
    python = venv / "bin" / "python"
    pip = venv / "bin" / "pip"
    if not python.exists():
        run("python", [sys.executable, "-m", "venv", str(venv)])
    run("python", [str(pip), "install", "-e", str(ROOT / "sdk" / "python")])
    run("python", [str(python), "-m", "py_compile", "contract.py"])
    run("python", [str(python), "-c", "import contract"])
    run("dotnet", ["dotnet", "build", "Contract.csproj", "--nologo"])
    run("java", ["mise", "exec", "java@temurin-17.0.20+8", "maven@3.9.11", "--", "mvn", "-q", "package", "-DskipTests"])


if __name__ == "__main__":
    main()
