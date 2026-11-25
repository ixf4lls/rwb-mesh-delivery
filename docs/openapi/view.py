"""Serve the OpenAPI files locally. Requires Python 3 and Ruby."""
import argparse
import json
from functools import partial
from http.server import SimpleHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
import subprocess

DIRECTORY = Path(__file__).resolve().parent


def read_spec(name):
    return subprocess.check_output([
        'ruby', '-ryaml', '-rjson', '-e',
        'puts JSON.generate(YAML.safe_load(File.read(ARGV.fetch(0))))',
        str(DIRECTORY / (name + '.yaml')),
    ])


class DocsHandler(SimpleHTTPRequestHandler):
    def do_GET(self):
        if self.path in ('/frontend.json', '/internal.json'):
            data = read_spec(self.path[1:-5])
            self.send_response(200)
            self.send_header('Content-Type', 'application/json; charset=utf-8')
            self.send_header('Content-Length', str(len(data)))
            self.end_headers()
            self.wfile.write(data)
            return
        if self.path == '/':
            self.path = '/index.html'
        super().do_GET()


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--port', type=int, default=8080)
    parser.add_argument('--export', type=Path, help='Write a standalone HTML document.')
    args = parser.parse_args()
    if args.export:
        specs = {name: json.loads(read_spec(name)) for name in ('frontend', 'internal')}
        payload = json.dumps(specs, ensure_ascii=False).replace('<', chr(92) + 'u003c')
        html = (DIRECTORY / 'index.html').read_text(encoding='utf-8')
        html = html.replace('<script>', '<script>\n  window.bundledSpecs = ' + payload + ';', 1)
        args.export.write_text(html, encoding='utf-8')
        raise SystemExit(0)
    with ThreadingHTTPServer(('127.0.0.1', args.port), partial(DocsHandler, directory=DIRECTORY)) as server:
        print(f'http://127.0.0.1:{server.server_port}', flush=True)
        server.serve_forever()
