from __future__ import annotations

import json
import urllib.error
import urllib.parse
import urllib.request
from dataclasses import dataclass
from typing import Any, Mapping, Optional


class Problem(RuntimeError):
    def __init__(self, value: Mapping[str, Any]):
        self.status = int(value.get("status", 0))
        self.code = str(value.get("code", "unknown_error"))
        self.message_id = str(value.get("message_id", "error.common.request_failed"))
        self.message = str(value.get("message", "Tvpn request failed"))
        super().__init__(f"{self.message} ({self.code}, HTTP {self.status})")


@dataclass(frozen=True)
class Response:
    status: int
    headers: Mapping[str, str]
    content: bytes

    @property
    def text(self) -> str:
        return self.content.decode("utf-8")

    def json(self) -> Any:
        return json.loads(self.content)


class _RedirectHandler(urllib.request.HTTPRedirectHandler):
    def __init__(self, token: str):
        self.token = token

    def redirect_request(self, req, fp, code, msg, headers, newurl):
        redirected = super().redirect_request(req, fp, code, msg, headers, newurl)
        if redirected is not None:
            if urllib.parse.urlsplit(req.full_url).hostname != urllib.parse.urlsplit(newurl).hostname:
                redirected.remove_header("Authorization")
            redirected.add_unredirected_header("Proxy-Authorization", f"Bearer {self.token}")
        return redirected


class Client:
    def __init__(self, base_url: str, token: str, timeout: float = 60):
        self.base_url = base_url.rstrip("/")
        self.token = token
        self.timeout = timeout
        self._opener = urllib.request.build_opener(_RedirectHandler(token))

    def request(
        self,
        method: str,
        url: str,
        *,
        headers: Optional[Mapping[str, str]] = None,
        data: Optional[bytes] = None,
        json_body: Any = None,
        upstream_proxy_id: Optional[str] = None,
        compatibility_mode: bool = False,
    ) -> Response:
        with self.session(url, upstream_proxy_id=upstream_proxy_id, compatibility_mode=compatibility_mode) as session:
            return session.request(method, url, headers=headers, data=data, json_body=json_body)

    def session(self, url: str, *, upstream_proxy_id: Optional[str] = None, compatibility_mode: bool = False) -> "Session":
        navigation = self._management_request(
            "POST",
            "/api/v1/proxy/contexts/",
            {
                "url": url,
                "upstream_proxy_id": upstream_proxy_id,
                "compatibility_mode": compatibility_mode,
            },
        )
        return Session(self, navigation["context"]["id"], url, navigation["route_url"])

    def get(self, url: str, **kwargs: Any) -> Response:
        return self.request("GET", url, **kwargs)

    def post(self, url: str, **kwargs: Any) -> Response:
        return self.request("POST", url, **kwargs)

    def _route_request(self, route_url: str, method: str, headers: Optional[Mapping[str, str]], data: Optional[bytes], json_body: Any) -> Response:
        if data is not None and json_body is not None:
            raise ValueError("data and json_body are mutually exclusive")
        request_headers = dict(headers or {})
        if json_body is not None:
            data = json.dumps(json_body).encode("utf-8")
            request_headers.setdefault("Content-Type", "application/json")
        request_headers["Proxy-Authorization"] = f"Bearer {self.token}"
        request = urllib.request.Request(route_url, data=data, headers=request_headers, method=method.upper())
        return self._open(request, management=False)

    def _management_request(self, method: str, path: str, body: Any = None) -> Any:
        data = None if body is None else json.dumps(body).encode("utf-8")
        headers = {"Authorization": f"Bearer {self.token}"}
        if data is not None:
            headers["Content-Type"] = "application/json"
        request = urllib.request.Request(self.base_url + path, data=data, headers=headers, method=method)
        response = self._open(request, management=True)
        return None if not response.content else response.json()

    def _open(self, request: urllib.request.Request, management: bool) -> Response:
        try:
            with self._opener.open(request, timeout=self.timeout) as response:
                return Response(response.status, dict(response.headers.items()), response.read())
        except urllib.error.HTTPError as error:
            content = error.read()
            if management or error.headers.get("Tvpn-Error-Code", ""):
                try:
                    raise Problem(json.loads(content)) from error
                except json.JSONDecodeError:
                    raise RuntimeError(f"Tvpn returned HTTP {error.code}") from error
            return Response(error.code, dict(error.headers.items()), content)


class Session:
    def __init__(self, client: Client, context_id: str, initial_url: str, initial_route: str):
        self.client = client
        self.context_id = context_id
        self.initial_url = initial_url
        self.initial_route = initial_route
        self.used_initial = False
        self.closed = False

    def request(self, method: str, url: str, *, headers: Optional[Mapping[str, str]] = None, data: Optional[bytes] = None, json_body: Any = None) -> Response:
        if self.closed:
            raise RuntimeError("Tvpn session is closed")
        if not self.used_initial and url == self.initial_url:
            route_url = self.initial_route
            self.used_initial = True
        else:
            navigation = self.client._management_request("POST", f"/api/v1/proxy/contexts/{urllib.parse.quote(self.context_id)}/navigate", {"url": url})
            route_url = navigation["route_url"]
        return self.client._route_request(route_url, method, headers, data, json_body)

    def get(self, url: str, **kwargs: Any) -> Response:
        return self.request("GET", url, **kwargs)

    def post(self, url: str, **kwargs: Any) -> Response:
        return self.request("POST", url, **kwargs)

    def close(self) -> None:
        if not self.closed:
            self.client._management_request("DELETE", f"/api/v1/proxy/contexts/{urllib.parse.quote(self.context_id)}")
            self.closed = True

    def __enter__(self) -> "Session":
        return self

    def __exit__(self, exc_type, exc_value, traceback) -> None:
        try:
            self.close()
        except Exception:
            if exc_type is None:
                raise
