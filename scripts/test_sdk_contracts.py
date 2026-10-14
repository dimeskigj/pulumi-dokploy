#!/usr/bin/env python3
"""Compile/import the manually maintained, offline GitHub SDK contract fixtures."""
import os
from pathlib import Path
import subprocess
import sys
import tempfile

ROOT = Path(__file__).resolve().parents[1]
FIXTURES = ROOT / "examples" / "sdk-contracts"


def runtime_environment():
    environment = {key: value for key, value in os.environ.items() if not key.startswith("DOKPLOY_")}
    environment["DOKPLOY_ACCEPTANCE"] = "0"
    return environment


def run(language, command, **kwargs):
    print(f"[{language}] {' '.join(map(str, command))}", flush=True)
    subprocess.run(list(map(str, command)), cwd=FIXTURES / language, check=True,
                   env=runtime_environment(), **kwargs)


def main():
    run("go", ["go", "test", "./..."])
    run("nodejs", ["npm", "install", "--package-lock=false", "--ignore-scripts", "--no-audit", "--no-fund"])
    run("nodejs", ["npx", "tsc", "--noEmit"])
    run("nodejs", ["node", "runtime-contract.mjs"])
    with tempfile.TemporaryDirectory(prefix="dokploy-sdk-contract-", dir="/tmp/opencode") as directory:
        venv = Path(directory) / "python-env"
        python = venv / "bin" / "python"
        pip = venv / "bin" / "pip"
        run("python", [sys.executable, "-m", "venv", str(venv)])
        run("python", [str(pip), "install", "-e", str(ROOT / "sdk" / "python")])
        run("python", [str(python), "-m", "py_compile", "contract.py"])
        run("python", [str(python), "-c", "import contract"])
    run("dotnet", ["dotnet", "build", "Contract.csproj", "--nologo"])
    run("java", ["mise", "exec", "java@temurin-17.0.20+8", "maven@3.9.11", "--", "mvn", "-q", "package", "-DskipTests"])
    with tempfile.TemporaryDirectory(prefix="dokploy-java-runtime-", dir="/tmp/opencode") as directory:
        classpath_file = Path(directory) / "classpath.txt"
        run("java", ["mise", "exec", "java@temurin-17.0.20+8", "maven@3.9.11", "--", "mvn", "-q",
                      "test-compile", "dependency:build-classpath", f"-Dmdep.outputFile={classpath_file}"])
        dependencies = classpath_file.read_text().strip()
        run("java", ["mise", "exec", "java@temurin-17.0.20+8", "--", "java", "-cp",
                      f"{FIXTURES / 'java' / 'target' / 'test-classes'}:{dependencies}", "RuntimeContract"])


if __name__ == "__main__":
    main()
