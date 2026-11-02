#!/usr/bin/env bash
# Single public entry point. Helpers live in lib/ and are not user commands.
set -Eeuo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
usage() {
  cat <<'HELP'
用法：bash scripts/deploy.sh <操作> [组件] [--pull]

  install [组件]              安装（默认 all）
  update [组件] [--pull]      更新当前源码；--pull 先 fast-forward 拉取 Git
  remote <SSH目标> <操作>    上传本机源码并在服务器 install 或 update
  auth on|off                开启/关闭聊天认证（需要 sudo）
  help                       显示帮助

组件：all、api、agent、frontend、nginx、mysql、relay、ios-app、wechat-miniprogram
all 包含 MySQL、Agent、API、前端与 Nginx，不包含中转机和移动客户端。
服务器安装/更新需要 Linux + sudo；iOS 在 macOS 执行，小程序在开发机执行。

常用命令：
  sudo bash scripts/deploy.sh update --pull
  sudo bash scripts/deploy.sh update api
  bash scripts/deploy.sh remote my-server update
  bash scripts/deploy.sh install ios-app

更新不重置数据库、已有环境文件或认证设置。
--pull 仅适用于服务器组件；要求 Git 工作区干净，默认分支 main。
自定义配置沿用 APP_DIR、DOMAIN、ENV_DIR、REMOTE_DIR、BRANCH 等环境变量。
HELP
}
fail() { echo "$*" >&2; echo '查看用法：bash scripts/deploy.sh help' >&2; exit 2; }
action="${1:-help}"
[[ $# == 0 ]] || shift
case "$action" in
  help|-h|--help)
    [[ $# == 0 ]] || fail 'help 不接受其他参数'
    usage;;
  install|update)
    component=all
    component_set=0
    pull_flag=0
    for arg in "$@"; do
      case "$arg" in
        --pull)
          [[ "$action" == update && "$pull_flag" == 0 ]] || fail '--pull 只能在 update 中使用一次'
          pull_flag=1;;
        all|api|agent|frontend|nginx|mysql|relay|ios-app|wechat-miniprogram)
          [[ "$component_set" == 0 ]] || fail '每次只指定一个组件；全站使用 all'
          component="$arg"; component_set=1;;
        *) fail "未知参数：$arg";;
      esac
    done
    [[ "$pull_flag" == 0 ]] || export PULL=1
    case "$component" in
      relay|ios-app|wechat-miniprogram)
        [[ "${PULL:-0}" != 1 ]] || fail '客户端/中转机请先自行拉取代码，再执行安装或更新'
        exec bash "$SCRIPT_DIR/lib/clients.sh" "$component";;
      *) exec bash "$SCRIPT_DIR/lib/server.sh" "$component" "$action";;
    esac;;
  remote)
    [[ $# == 2 ]] || fail '用法：bash scripts/deploy.sh remote <SSH目标> install|update'
    [[ "$2" == install || "$2" == update ]] || fail '远程操作只支持 install 或 update'
    exec bash "$SCRIPT_DIR/lib/remote.sh" "$1" "$2";;
  auth)
    [[ $# == 1 && ( "$1" == on || "$1" == off ) ]] || fail '用法：sudo bash scripts/deploy.sh auth on|off'
    exec bash "$SCRIPT_DIR/lib/auth.sh" "$1";;
  *) fail "未知操作：$action";;
esac
