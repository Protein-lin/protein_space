import importlib.util
import os
from pathlib import Path
import subprocess
import tarfile
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]


class DeploymentTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="protein-deploy-test-")
        self.base = Path(self.temp.name).resolve()
        self.addCleanup(self.temp.cleanup)
        self.env = dict(os.environ, APP_DIR=str(self.base / "app"),
                        ENV_DIR=str(self.base / "env"),
                        SYSTEMD_DIR=str(self.base / "systemd"),
                        NGINX_DIR=str(self.base / "nginx"),
                        LOCK_FILE=str(self.base / "deploy.lock"),
                        CERT_DIR=str(self.base / "certs"), DOMAIN="chat.example.com",
                        PULL="0")

    def run_script(self, script, *args, **env):
        return subprocess.run(["bash", str(ROOT / script), *args],
                              env=dict(self.env, **env), text=True,
                              stdout=subprocess.PIPE, stderr=subprocess.PIPE)

    def test_http_routes_and_domain(self):
        result = self.run_script("scripts/lib/nginx.sh")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("listen 80;", result.stdout)
        self.assertNotIn("ssl_certificate", result.stdout)
        self.assertIn("protein-space-api.conf", result.stdout)
        self.assertIn("try_files $uri $uri/ /index.html", result.stdout)
        result = self.run_script("scripts/lib/nginx.sh", DOMAIN="www.example.com")
        self.assertNotIn("www.www.", result.stdout)

    def test_https_routes_and_redirect(self):
        certs = Path(self.env["CERT_DIR"])
        certs.mkdir()
        for name in ["fullchain.pem", "privkey.pem"]:
            (certs / name).touch()
        result = self.run_script("scripts/lib/nginx.sh")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("listen 443 ssl http2;", result.stdout)
        self.assertIn("return 301 https://$host$request_uri", result.stdout)
        self.assertIn("/.well-known/acme-challenge/", result.stdout)
        self.assertEqual(result.stdout.count("include "), 1)

    def test_reject_config_injection(self):
        self.assertNotEqual(self.run_script("scripts/lib/nginx.sh", DOMAIN="x; return 200;").returncode, 0)
        self.assertNotEqual(self.run_script("scripts/lib/nginx.sh", APP_DIR="/tmp/a b").returncode, 0)
        self.assertNotEqual(self.run_script("scripts/deploy.sh", "-oProxyCommand=bad").returncode, 0)

    def test_api_route_inventory_and_stream_settings(self):
        import re
        handlers = re.findall(r'mux.HandleFunc\("(/api/[^\"]+)"', (ROOT / "api/main.go").read_text())
        config = (ROOT / "nginx/api-locations.conf").read_text()
        for route in handlers:
            self.assertIn(route, config)
        for setting in ["location ^~ /api/", "proxy_pass http://127.0.0.1:8080;",
                        "proxy_buffering off;", "proxy_cache off;", "client_max_body_size 24m;",
                        "location ^~ /v1/ { return 404; }"]:
            self.assertIn(setting, config)

    def test_package_excludes_private_files(self):
        spec = importlib.util.spec_from_file_location("package", ROOT / "scripts/lib/package.py")
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        module.ROOT = self.base / "source"
        for name in ["api/main.go", "api/waf_rules_prompt.txt", "api/private.txt", "api/private/.env", ".env", "111.md", "relay/proxy.py",
                     "scripts/lib/clients.sh", "wechat-miniprogram/project.private.config.json",
                     "wechat-miniprogram/app.json", "ios-app/build/leaked.swift", "scripts/deploy.sh"]:
            path = module.ROOT / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text("test")
        (module.ROOT / "api/linked.go").symlink_to(module.ROOT / ".env")
        archive = self.base / "source.tar.gz"
        module.package(archive)
        with tarfile.open(archive) as package:
            self.assertEqual(set(package.getnames()), {"api/main.go", "api/waf_rules_prompt.txt", "scripts/lib/clients.sh", "wechat-miniprogram/app.json", "scripts/deploy.sh"})

    def mock_commands(self):
        app = Path(self.env["APP_DIR"])
        for directory in ["api", "agent", "bin", "nginx", "scripts/lib"]:
            (app / directory).mkdir(parents=True, exist_ok=True)
        for name in ["scripts/lib/nginx.sh", "nginx/api-locations.conf"]:
            (app / name).write_text((ROOT / name).read_text())
        config = Path(self.env["ENV_DIR"])
        config.mkdir()
        (config / "agent.env").write_text("AGENT_UPSTREAM_URL=https://provider.example/v1/chat/completions\nAGENT_API_KEY=test\n")
        (config / "api.env").write_text("MYSQL_DSN=preserved\nAPP_ENCRYPTION_KEY=preserved\n")
        (app / ".env").write_text("MYSQL_PASSWORD=test-keep-existing\n")
        commands = self.base / "commands"
        commands.mkdir()
        log = self.base / "commands.log"
        stub = '''#!/usr/bin/env bash
set -eu
name="${0##*/}"
printf '%s %s\\n' "$name" "$*" >> "$MOCK_LOG"
case "$name" in
  git) [[ "$*" != *"status --porcelain"* || "${DIRTY_GIT:-0}" != 1 ]] || echo " M local-file";;
  uname) echo Linux;;
  id) echo 0;;
  go)
    if [[ "$1" == test && "${FAIL_TEST:-0}" == 1 ]]; then exit 1; fi
    if [[ "$1" == build ]]; then
      while [[ $# -gt 0 ]]; do
        if [[ "$1" == -o ]]; then shift; printf 'new-binary' > "$1"; break; fi
        shift
      done
    fi;;
  nginx) [[ "${FAIL_NGINX:-0}" != 1 ]];;
  curl) [[ "${FAIL_HEALTH:-0}" != 1 ]];;
esac
'''
        for name in ["uname", "id", "flock", "apt-get", "systemctl", "go", "curl", "nginx", "docker", "sleep", "git"]:
            path = commands / name
            path.write_text(stub)
            path.chmod(0o755)
        self.env.update(PATH=str(commands) + os.pathsep + os.environ["PATH"], MOCK_LOG=str(log))
        return app, log

    def test_install_all_preserves_config_and_builds_before_restart(self):
        app, log = self.mock_commands()
        result = self.run_script("scripts/deploy.sh", "install")
        self.assertEqual(result.returncode, 0, result.stderr)
        lines = log.read_text()
        self.assertLess(lines.rindex("go build"), lines.index("systemctl restart protein-agent"))
        self.assertIn("nginx -t", lines)
        self.assertIn("systemctl reload nginx", lines)
        self.assertEqual((Path(self.env["ENV_DIR"]) / "api.env").read_text(), "MYSQL_DSN=preserved\nAPP_ENCRYPTION_KEY=preserved\n")
        self.assertEqual((app / "bin/api").read_text(), "new-binary")

    def test_failed_build_does_not_replace_running_binary(self):
        app, log = self.mock_commands()
        (app / "bin/api").write_text("old-binary")
        result = self.run_script("scripts/deploy.sh", "update", "api", FAIL_TEST="1")
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual((app / "bin/api").read_text(), "old-binary")
        self.assertNotIn("systemctl restart", log.read_text())

    def test_failed_health_restores_binary(self):
        app, log = self.mock_commands()
        (app / "bin/api").write_text("old-binary")
        result = self.run_script("scripts/deploy.sh", "update", "api", FAIL_HEALTH="1")
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual((app / "bin/api").read_text(), "old-binary")
        self.assertEqual(log.read_text().count("systemctl restart protein-api"), 2)

    def test_invalid_nginx_restores_config(self):
        self.mock_commands()
        nginx = Path(self.env["NGINX_DIR"])
        site = nginx / "sites-available/protein-space"
        site.parent.mkdir(parents=True)
        site.write_text("old-config")
        result = self.run_script("scripts/deploy.sh", "update", "nginx", FAIL_NGINX="1")
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(site.read_text(), "old-config")
        self.assertFalse((nginx / "sites-enabled/protein-space").exists())

    def test_fresh_install_generates_secrets_and_stops_for_provider_config(self):
        app, log = self.mock_commands()
        (app / ".env").unlink()
        for name in ["api.env", "agent.env"]:
            (Path(self.env["ENV_DIR"]) / name).unlink()
        result = self.run_script("scripts/deploy.sh", "install")
        self.assertEqual(result.returncode, 2, result.stderr)
        secret = (app / ".env").read_text().split("MYSQL_PASSWORD=")[1].strip()
        self.assertEqual(len(secret), 64)
        self.assertEqual((app / ".env").stat().st_mode & 0o777, 0o600)
        self.assertIn(secret, (Path(self.env["ENV_DIR"]) / "api.env").read_text())
        self.assertNotIn("systemctl", log.read_text())

    def test_chat_auth_switch_preserves_existing_settings(self):
        self.mock_commands()
        config = Path(self.env["ENV_DIR"]) / "api.env"
        config.write_text("MYSQL_DSN=keep\nAUTH_REQUIRED=true\nAPP_ENCRYPTION_KEY=keep\n")
        for mode, expected in [("off", "false"), ("off", "false"), ("on", "true")]:
            result = self.run_script("scripts/deploy.sh", "auth", mode)
            self.assertEqual(result.returncode, 0, result.stderr)
            text = config.read_text()
            self.assertEqual(text.count("AUTH_REQUIRED="), 1)
            self.assertIn("AUTH_REQUIRED=" + expected, text)
            self.assertIn("MYSQL_DSN=keep", text)
            self.assertIn("APP_ENCRYPTION_KEY=keep", text)
            self.assertEqual(config.stat().st_mode & 0o777, 0o600)


    def test_help_and_invalid_commands_have_no_side_effects(self):
        app, log = self.mock_commands()
        for args in [(), ("help",), ("--help",)]:
            result = self.run_script("scripts/deploy.sh", *args)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertIn("update", result.stdout)
        for args in [("update", "typo"), ("install", "--pull"),
                     ("update", "api", "agent"), ("update", "--pull", "--pull"),
                     ("update", "relay", "--pull"), ("auth",), ("auth", "invalid"),
                     ("remote",), ("remote", "host", "delete"),
                     ("remote", "-oProxyCommand=bad", "update")]:
            result = self.run_script("scripts/deploy.sh", *args)
            self.assertEqual(result.returncode, 2, (args, result.stderr))
        self.assertFalse(log.exists(), "invalid commands must not run system commands")

    def test_update_pull_is_fast_forward_and_before_build(self):
        app, log = self.mock_commands()
        (app / ".git").mkdir()
        result = self.run_script("scripts/deploy.sh", "update", "api", "--pull")
        self.assertEqual(result.returncode, 0, result.stderr)
        lines = log.read_text()
        self.assertIn("pull --ff-only origin main", lines)
        self.assertLess(lines.index("pull --ff-only"), lines.index("go test"))
        log.unlink()
        result = self.run_script("scripts/deploy.sh", "update", "--pull", "api", DIRTY_GIT="1")
        self.assertNotEqual(result.returncode, 0)
        self.assertNotIn("pull --ff-only", log.read_text())
        self.assertNotIn("go test", log.read_text())

    def test_plain_update_does_not_pull_or_change_auth(self):
        app, log = self.mock_commands()
        config = Path(self.env["ENV_DIR"]) / "api.env"
        config.write_text("AUTH_REQUIRED=true\nMYSQL_DSN=preserved\n")
        result = self.run_script("scripts/deploy.sh", "update")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertNotIn("git ", log.read_text())
        self.assertEqual(config.read_text(), "AUTH_REQUIRED=true\nMYSQL_DSN=preserved\n")

    def test_remote_upload_calls_unified_entrypoint(self):
        app, log = self.mock_commands()
        commands = self.base / "commands"
        ssh = commands / "ssh"
        ssh.write_text("""#!/usr/bin/env bash
set -eu
printf 'ssh %s\\n' "$*" >> "$MOCK_LOG"
case "$*" in
  *mktemp*) echo /tmp/protein-space-source.TEST1234.tar.gz;;
  *'bash -s'*) cat > "$MOCK_LOG.remote";;
esac
""")
        ssh.chmod(0o755)
        scp = commands / "scp"
        scp.write_text('#!/usr/bin/env bash\nprintf "scp %s\\n" "$*" >> "$MOCK_LOG"\n')
        scp.chmod(0o755)
        result = self.run_script("scripts/deploy.sh", "remote", "test-server", "update")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("scp ", log.read_text())
        remote = Path(str(log) + ".remote").read_text()
        self.assertIn('PULL=0 bash "$app_dir/scripts/deploy.sh" "$action"', remote)
        self.assertLess(remote.index('tar --no-same-owner'), remote.index('PULL=0 bash'))
        self.assertIn("backups/source-", remote)
        self.assertNotIn("StrictHostKeyChecking=no", log.read_text())

    def test_miniprogram_dispatch_supports_check_only(self):
        app, log = self.mock_commands()
        node = self.base / "commands/node"
        node.write_text('#!/usr/bin/env bash\nprintf "node %s\\n" "$*" >> "$MOCK_LOG"\n')
        node.chmod(0o755)
        result = self.run_script("scripts/deploy.sh", "update", "wechat-miniprogram", CHECK_ONLY="1")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("小程序本地测试通过", result.stdout)
        self.assertIn("node --test", log.read_text())
        self.assertIn(str(ROOT / "wechat-miniprogram"), log.read_text())
        self.assertNotIn("systemctl", log.read_text())

    def test_only_one_public_shell_script(self):
        self.assertEqual([p.name for p in (ROOT / "scripts").glob("*.sh")], ["deploy.sh"])


if __name__ == "__main__":
    unittest.main()
