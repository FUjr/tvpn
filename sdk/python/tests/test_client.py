import unittest

from tvpn import Client, Response


class SessionTest(unittest.TestCase):
    def test_persistent_session_reuses_context_and_navigates(self):
        client = Client("https://vpn.example.com", "tvpn_pat_test")
        management = []
        routes = []

        def management_request(method, path, body=None):
            management.append((method, path, body))
            if path.endswith("/contexts/"):
                return {"context": {"id": "persistent"}, "route_url": "https://route-one.proxy.example.com/login"}
            if path.endswith("/navigate"):
                return {"route_url": "https://route-two.proxy.example.com/devices"}
            return None

        def route_request(route_url, method, headers, data, json_body):
            routes.append(route_url)
            return Response(200, {}, b"{}")

        client._management_request = management_request
        client._route_request = route_request
        with client.session("https://api.example.com/login") as session:
            session.post("https://api.example.com/login", json_body={"user": "test"})
            session.get("https://api.example.com/devices")

        self.assertEqual(routes, ["https://route-one.proxy.example.com/login", "https://route-two.proxy.example.com/devices"])
        self.assertEqual(management[-1][:2], ("DELETE", "/api/v1/proxy/contexts/persistent"))


if __name__ == "__main__":
    unittest.main()
