#!/usr/bin/env python3
"""Allowlisted source package; never include private configuration or notes."""
import argparse
from pathlib import Path
import tarfile

ROOT = Path(__file__).resolve().parent.parent
TOP = {"README.md", "DEPLOYMENT.md", "go.mod", "docker-compose.yml"}
DIRS = {"api", "agent", "frontend", "nginx", "mysql", "scripts", "ios-app", "wechat-miniprogram"}
SKIP = {"build", "DerivedData", "xcuserdata", "__pycache__", "node_modules", "bin", "backups"}
SUFFIXES = {".go", ".mod", ".sum", ".js", ".cjs", ".css", ".html", ".svg", ".json", ".wxss", ".wxml", ".sql", ".sh", ".py", ".conf", ".md", ".swift", ".pbxproj", ".xcscheme"}

def package(output):
    files = []
    for path in sorted(ROOT.rglob("*")):
        relative = path.relative_to(ROOT)
        if path.is_symlink() or not path.is_file():
            continue
        if any(part.startswith(".") or part in SKIP for part in relative.parts):
            continue
        if path.name == "project.private.config.json":
            continue
        if str(relative) in TOP or (relative.parts[0] in DIRS and (path.suffix in SUFFIXES or path.name == "Dockerfile")):
            files.append(path)
    with tarfile.open(output, "w:gz") as archive:
        for path in files:
            archive.add(path, arcname=str(path.relative_to(ROOT)), recursive=False)
    return len(files)

if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("output", type=Path)
    args = parser.parse_args()
    print(f"已打包 {package(args.output)} 个源码文件：{args.output}")
