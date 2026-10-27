#!/usr/bin/env python3
"""Magnific 素材上传和团队积分对账工具，仅使用 Python 3 标准库。"""

import argparse
import datetime
import http.client
import json
import math
import mimetypes
import os
from pathlib import Path
import stat
import sys
import urllib.error
import urllib.parse
import urllib.request


MAX_SIZE = 1024 ** 3
MIME_TYPES = (
    "image/png", "image/jpeg", "image/webp", "video/mp4", "video/webm",
    "video/quicktime", "audio/mpeg", "audio/wav", "audio/mp4",
    "audio/webm", "audio/ogg",
)
# 统一平台相关的 MIME 别名；WebM 默认按视频处理，音频可用 --content-type 指定。
MIME_BY_SUFFIX = {
    ".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg",
    ".webp": "image/webp", ".mp4": "video/mp4", ".webm": "video/webm",
    ".mov": "video/quicktime", ".mp3": "audio/mpeg", ".wav": "audio/wav",
    ".m4a": "audio/mp4", ".ogg": "audio/ogg",
}
LIFETIME = "素材 URL 约一天有效，过期后用 uploads 刷新；文件约七天后可被清理，请自行保留原文件。"


class CLIError(Exception):
    """可直接向用户显示的中文错误。"""


class ChineseParser(argparse.ArgumentParser):
    def __init__(self, *args, **kwargs):
        kwargs["add_help"] = False
        super().__init__(*args, **kwargs)
        self._positionals.title = "位置参数"
        self._optionals.title = "选项"
        self.add_argument("-h", "--help", action="help", help="显示帮助并退出")

    def format_usage(self):
        return super().format_usage().replace("usage: ", "用法：")

    def format_help(self):
        return super().format_help().replace("usage: ", "用法：")

    def error(self, message):
        for source, target in (
            ("the following arguments are required:", "缺少必填参数："),
            ("unrecognized arguments:", "无法识别的参数："),
            ("invalid choice:", "不支持的选项："),
            ("choose from", "可选值"),
            ("expected one argument", "需要一个参数值"),
            ("expected at least one argument", "至少需要一个参数值"),
            ("argument ", "参数 "),
        ):
            message = message.replace(source, target)
        self.print_usage(sys.stderr)
        self.exit(2, "错误：" + message + "\n")


class NoRedirect(urllib.request.HTTPRedirectHandler):
    # 防止凭据随重定向转发，也避免 POST/PUT 被隐式重新发送。
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


def date_value(value):
    try:
        parsed = datetime.datetime.strptime(value, "%Y-%m-%d").date()
    except ValueError:
        raise argparse.ArgumentTypeError("日期必须为有效的 YYYY-MM-DD") from None
    if parsed.isoformat() != value:
        raise argparse.ArgumentTypeError("日期必须为 YYYY-MM-DD")
    return value


def group_id(value):
    try:
        result = int(value)
    except ValueError:
        raise argparse.ArgumentTypeError("组 ID 必须为正整数") from None
    if result < 1:
        raise argparse.ArgumentTypeError("组 ID 必须为正整数")
    return result


def timeout_value(value):
    try:
        result = float(value)
    except ValueError:
        raise argparse.ArgumentTypeError("超时秒数必须为正数") from None
    if not math.isfinite(result) or result <= 0:
        raise argparse.ArgumentTypeError("超时秒数必须为有限正数")
    return result


def parse_args(argv):
    common = argparse.ArgumentParser(add_help=False)
    common.add_argument("--base-url", default=argparse.SUPPRESS,
                        help="API 基础地址，默认 https://api.magnific.com")
    common.add_argument("--timeout", type=timeout_value, default=argparse.SUPPRESS,
                        help="每次网络操作超时秒数，默认 120")
    common.add_argument("--output", metavar="JSON文件", default=argparse.SUPPRESS,
                        help="同时将成功结果保存为 UTF-8 JSON 文件")
    parser = ChineseParser(
        description="Magnific 素材上传与积分对账；凭据仅从 MAGNIFIC_API_KEY 环境变量读取。",
        epilog=LIFETIME + " 积分统计是聚合账单，不是逐任务扣费或余额。所有请求失败均不自动重试。",
        parents=[common],
    )
    commands = parser.add_subparsers(dest="command", required=True,
                                     title="子命令", metavar="{upload,uploads,credits}")
    upload_parser = commands.add_parser(
        "upload", parents=[common], help="上传 1–14 个本地素材文件",
        description="依次上传本地文件，每个文件最大 1 GiB。" + LIFETIME,
    )
    upload_parser.add_argument("files", nargs="+", metavar="文件", help="本地素材文件路径")
    upload_parser.add_argument(
        "--content-type", action="append", choices=MIME_TYPES, metavar="MIME",
        help="覆盖自动识别的 MIME；一次用于全部文件，或按文件顺序逐个指定。允许：" + ", ".join(MIME_TYPES),
    )
    commands.add_parser("uploads", parents=[common], help="列出素材并刷新素材 URL",
                        description="GET /v1/ai/uploads，获取新签名的素材 URL。" + LIFETIME)
    credits_parser = commands.add_parser(
        "credits", parents=[common], help="查询团队积分消费聚合账单",
        description="返回原始聚合统计，不是逐任务扣费或余额。仅限 Business / Enterprise，查询不消耗积分。",
        epilog="不指定日期时由服务端默认最近七天；Business 最多 180 天，Enterprise 最多 365 天。"
               "同一工具分类下 tool_name 不同的消费行应累计，不能去重。",
    )
    credits_parser.add_argument("--start-date", type=date_value, help="开始日期 YYYY-MM-DD")
    credits_parser.add_argument("--end-date", type=date_value, help="结束日期 YYYY-MM-DD")
    credits_parser.add_argument("--granularity", choices=("day", "week", "month", "year"),
                                default="day", help="聚合粒度，默认 day（日）")
    credits_parser.add_argument("--api-key-id", action="append", help="API 密钥 ID 筛选，可重复；不是密钥本身")
    credits_parser.add_argument("--project-reference", action="append", help="项目 UUID 筛选，可重复")
    credits_parser.add_argument("--group-id", action="append", type=group_id, help="团队组 ID 筛选，可重复")
    args = parser.parse_args(argv)
    if args.command == "credits" and args.start_date and args.end_date and args.start_date > args.end_date:
        parser.error("开始日期不能晚于结束日期")
    return args


class Client:
    def __init__(self, base_url, api_key, timeout):
        parsed = urllib.parse.urlsplit(base_url)
        if (parsed.scheme not in ("http", "https") or not parsed.hostname
                or parsed.username or parsed.password or parsed.query or parsed.fragment):
            raise CLIError("API 基础地址必须为不含凭据、查询参数或片段的 HTTP(S) 地址")
        self.base_url = base_url.rstrip("/")
        self.api_key = api_key
        self.timeout = timeout
        self.opener = urllib.request.build_opener(NoRedirect())

    def send(self, request, operation, read_json=True):
        try:
            with self.opener.open(request, timeout=self.timeout) as response:
                if not 200 <= response.status < 300:
                    raise CLIError(operation + "失败：HTTP " + str(response.status) + "；未自动重试")
                if read_json:
                    return json.load(response)
        except urllib.error.HTTPError as exc:
            status = exc.code
            exc.close()
            # 不回显上游错误正文、URL 或 headers，避免包含凭据或签名。
            raise CLIError(operation + "失败：HTTP " + str(status) + "；未自动重试") from None
        except (urllib.error.URLError, http.client.HTTPException, OSError, ValueError) as exc:
            if isinstance(exc, (json.JSONDecodeError, UnicodeError)):
                raise CLIError(operation + "失败：响应不是有效 JSON；未自动重试") from None
            raise CLIError(operation + "失败：网络、文件读取或请求格式错误；未自动重试") from None

    def api(self, method, path, payload=None):
        data = None if payload is None else json.dumps(payload).encode("utf-8")
        headers = {"X-Magnific-API-Key": self.api_key, "Accept": "application/json"}
        if data is not None:
            headers["Content-Type"] = "application/json"
        return self.send(urllib.request.Request(self.base_url + path, data=data,
                                               headers=headers, method=method), "API 请求")

    def upload(self, paths, content_types):
        if len(paths) > 14:
            raise CLIError("一次只能上传 1–14 个文件")
        if content_types and len(content_types) not in (1, len(paths)):
            raise CLIError("--content-type 必须只指定一次，或与文件数量一致并按顺序指定")
        files = []
        for index, name in enumerate(paths):
            path = Path(name)
            info = path.stat()
            if not stat.S_ISREG(info.st_mode):
                raise CLIError("上传素材必须是普通文件")
            if info.st_size > MAX_SIZE:
                raise CLIError("每个文件最大 1 GiB")
            mime = (content_types[0 if len(content_types) == 1 else index] if content_types
                    else MIME_BY_SUFFIX.get(path.suffix.lower()) or mimetypes.guess_type(str(path))[0])
            if mime not in MIME_TYPES:
                raise CLIError("不支持此文件的 MIME 类型；用 upload --help 查看允许类型，可用 --content-type 指定")
            files.append((path, mime))
        result = self.api("POST", "/v1/ai/uploads/request-url",
                          {"files": [{"content_type": mime} for _, mime in files]})
        entries = result.get("files") if isinstance(result, dict) else None
        if not isinstance(entries, list) or len(entries) != len(files):
            raise CLIError("上传签名响应中的文件数量不匹配；未上传，可用 uploads 检查素材")
        # 先检查整批响应，避免结构错误时只完成一部分上传。
        for entry in entries:
            if (not isinstance(entry, dict)
                    or not all(isinstance(entry.get(key), str) and entry[key]
                               for key in ("upload_url", "file_id", "asset_url"))
                    or not isinstance(entry.get("headers"), dict)
                    or not all(isinstance(key, str) and isinstance(value, str)
                               for key, value in entry["headers"].items())
                    or not isinstance(entry.get("asset_url_expires_in"), int)):
                raise CLIError("上传签名响应缺少必要字段；未上传")
            parsed = urllib.parse.urlsplit(entry["upload_url"])
            if parsed.scheme not in ("http", "https") or not parsed.hostname:
                raise CLIError("上传签名地址必须为 HTTP(S) 地址；未上传")
            if any(key.lower() == "x-magnific-api-key" or self.api_key in value
                   for key, value in entry["headers"].items()):
                raise CLIError("上传签名 headers 含 API 密钥；拒绝向存储地址发送密钥")
        output = []
        for (path, _), entry in zip(files, entries):
            with path.open("rb") as source:
                size = os.fstat(source.fileno()).st_size
                if size > MAX_SIZE:
                    raise CLIError("素材大小已变化并超过 1 GiB；停止上传，可用 uploads 检查已完成素材")
                headers = dict(entry["headers"])
                content_length = next((value for key, value in headers.items()
                                       if key.lower() == "content-length"), None)
                if content_length is None:
                    headers["Content-Length"] = str(size)
                elif content_length != str(size):
                    raise CLIError("签名 Content-Length 与文件大小不一致；停止上传，可用 uploads 检查已完成素材")
                # urllib 按块读取文件对象；仅使用存储签名 headers，不复用 API headers。
                request = urllib.request.Request(entry["upload_url"], data=source,
                                                 headers=headers, method="PUT")
                self.send(request, "素材上传（此前素材可能已上传，可用 uploads 检查）", read_json=False)
            output.append({key: entry[key] for key in ("file_id", "asset_url", "asset_url_expires_in")})
        return {"files": output}


def main(argv=None):
    args = parse_args(argv)
    try:
        api_key = os.environ.get("MAGNIFIC_API_KEY", "").strip()
        if not api_key:
            raise CLIError("请先设置 MAGNIFIC_API_KEY 环境变量")
        client = Client(getattr(args, "base_url", "https://api.magnific.com"),
                        api_key, getattr(args, "timeout", 120))
        if args.command == "upload":
            result = client.upload(args.files, args.content_type)
        elif args.command == "uploads":
            result = client.api("GET", "/v1/ai/uploads")
        else:
            payload = {"granularity": args.granularity}
            for name in ("start_date", "end_date"):
                if getattr(args, name) is not None:
                    payload[name] = getattr(args, name)
            for name, target in (("api_key_id", "api_key_ids"),
                                 ("project_reference", "project_references"), ("group_id", "group_ids")):
                if getattr(args, name) is not None:
                    payload[target] = getattr(args, name)
            result = client.api("POST", "/v1/analytics/team-credit-usage", payload)
        text = json.dumps(result, ensure_ascii=False, indent=2) + "\n"
        if getattr(args, "output", None):
            Path(args.output).write_text(text, encoding="utf-8")
        sys.stdout.write(text)
        return 0
    except CLIError as exc:
        print("错误：" + str(exc), file=sys.stderr)
    except (OSError, ValueError):
        print("错误：无法读取素材、写入结果文件或处理请求；请检查路径、权限和参数", file=sys.stderr)
    except KeyboardInterrupt:
        print("错误：操作已中断；未自动重试，可用 uploads 检查已完成素材", file=sys.stderr)
    return 1


if __name__ == "__main__":
    sys.exit(main())
