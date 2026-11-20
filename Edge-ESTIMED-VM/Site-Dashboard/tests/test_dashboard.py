"""Smoke-test the dashboard and collector with a local oneM2M fixture."""

from datetime import datetime, timezone
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import threading
import time
import unittest
from urllib.error import HTTPError
from urllib.request import ProxyHandler, build_opener

import factory_c_address
from core import Store, load_config
from server import Dashboard


ROOT = Path(__file__).resolve().parents[1]
ANALYSIS = '/SiteB/ANALYSIS-AE-3/cnt-verdict'
JUDGE = '/SiteB/JUDGE-AE-3/cnt-judgement'
ROBOT = 'siteC_AMR_2'


class OneM2MFixture(BaseHTTPRequestHandler):
    def log_message(self, *args):
        pass

    def reply(self, status, rsc, data):
        wire = json.dumps(data).encode()
        self.send_response(status)
        self.send_header('Content-Type', 'application/json')
        self.send_header('Content-Length', str(len(wire)))
        self.send_header('X-M2M-RSC', rsc)
        self.send_header('X-M2M-RI', self.headers.get('X-M2M-RI', ''))
        self.end_headers()
        self.wfile.write(wire)

    def do_GET(self):
        if self.headers.get('X-M2M-Origin') != 'CFixture':
            self.reply(403, '4103', {})
            return
        now = datetime.now(timezone.utc).strftime('%Y%m%dT%H%M%S,%f')
        if self.path == '/SiteB':
            self.reply(200, '2000', {'m2m:cb': {'ty': 5, 'csi': '/example-cse'}})
            return
        containers = {
            ANALYSIS: ('verdict', {'type': 'amr-verdict', 'verdict': 'ANOMALY', 'robot': ROBOT,
                                  'site': 'siteC', 'pose': {'x': 5, 'y': 4}}),
            JUDGE: ('judgement', {'zones': [{'x1': 9, 'y1': 3, 'x2': 11, 'y2': 5}]}),
        }
        for channel in ('status', 'telemetry'):
            containers['/SiteB/'+ROBOT+'/'+channel] = (
                channel, {'robot': ROBOT, 'state': 'idle', 't_sim': 1,
                          'pose': {'x': 5, 'y': 4, 'yaw': 0}})
        for path, (name, content) in containers.items():
            if self.path == path:
                self.reply(200, '2000', {'m2m:cnt': {'ty': 3, 'ri': 'cnt-'+name}})
                return
            if self.path == path+'/la':
                self.reply(200, '2000', {'m2m:cin': {'ty': 4, 'ri': 'cin-'+name,
                    'pi': 'cnt-'+name, 'ct': now, 'st': 1, 'con': content}})
                return
        self.reply(404, '4004', {})

    def do_POST(self):
        # Allow the test to demonstrate GET reception during subscription failure.
        self.reply(500, '5204', {'m2m:dbg': 'Fixture subscription verification is unavailable.'})


class DashboardTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.cse = ThreadingHTTPServer(('127.0.0.1', 0), OneM2MFixture)
        self.start_server(self.cse)
        self.source = f'http://127.0.0.1:{self.cse.server_port}/SiteB'
        config = json.loads((ROOT/'config.example.json').read_text())
        config.update(cse_url=self.source, origin='CFixture')
        config['factory_c']['local_url'] = self.source
        self.config_path = Path(self.tmp.name)/'config.json'
        self.config_path.write_text(json.dumps(config))
        self.cfg = load_config(self.config_path)
        self.dashboard = Dashboard({**self.cfg, 'port': 0})
        self.start_server(self.dashboard)
        self.addCleanup(self.dashboard.stop)
        self.base = f'http://127.0.0.1:{self.dashboard.server_port}'
        self.opener = build_opener(ProxyHandler({}))

    def start_server(self, server):
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        self.addCleanup(server.server_close)
        self.addCleanup(thread.join, 3)
        self.addCleanup(server.shutdown)

    def get(self, path):
        with self.opener.open(self.base+path, timeout=3) as response:
            return response.read()

    def read_json(self, path):
        return json.loads(self.get(path))

    def test_static_and_configuration(self):
        self.assertEqual(self.read_json('/health')['status'], 'ready')
        self.assertIn(b'EdgeWay', self.get('/'))
        page = self.get('/tests/network-status')
        self.assertIn(b'Outage duration', page)
        self.assertIn(b'Lost data count', page)
        js = self.get('/tests/network-status.js')
        self.assertIn(b'outageDurationSeconds: 78', js)
        self.assertNotIn(b'fetch(', js)
        for factory in ('A', 'B', 'C'):
            self.assertTrue(self.get(f'/factory-maps/site{factory}.png').startswith(b'\x89PNG'))
            self.assertIn(b'origin: [0, 0, 0]', self.get(f'/factory-maps/site{factory}.yaml'))
        self.assertTrue(self.get('/fonts/PretendardVariable.woff2').startswith(b'wOF2'))
        selection = self.read_json('/api/factory-c/address')
        self.assertEqual(selection['url'], self.source)
        self.assertEqual([option['id'] for option in selection['options']], ['local'])
        self.assertFalse(self.dashboard.demo_commands.config.get('enabled', False))
        with self.assertRaises(HTTPError) as missing:
            self.get('/tests/does-not-exist')
        self.assertEqual(missing.exception.code, 404)
        self.assertEqual(self.read_json('/health')['status'], 'ready')

    def test_collector_and_ae_values(self):
        result = subprocess.run([sys.executable, str(ROOT/'watch_isaac_cse.py'),
                                 '--config', str(self.config_path), '--site', 'C',
                                 '--robots', ROBOT, '--once'], capture_output=True, text=True, timeout=12)
        self.assertEqual(result.returncode, 0, result.stdout+result.stderr)
        records = self.read_json('/api/events?robot='+ROBOT)['events']
        self.assertEqual({e['resource_path'] for e in records}, {ROBOT+'/status', ROBOT+'/telemetry'})
        self.assertEqual({e['source'] for e in records}, {self.source})
        until = time.monotonic()+8
        while time.monotonic()<until:
            floor = self.read_json('/api/floor-judgement')
            analysis = self.read_json('/api/analysis-verdict')
            if floor['zones'] and analysis['suspects']:
                break
            time.sleep(.1)
        self.assertEqual(floor['zones'][0]['x1'], 9)
        self.assertEqual(analysis['suspects'][0]['robot_id'], ROBOT)

    def test_mn_is_opt_in_and_invalid_urls_rejected(self):
        settings = {**self.cfg, 'factory_c': {**self.cfg['factory_c'],
                    'mn_url': 'https://factory-c.example.org/cse'}}
        store = Store(Path(self.tmp.name)/'selector.sqlite3')
        snap = factory_c_address.snapshot(settings, store)
        saved = factory_c_address.save(settings, store,
                    {'preset': 'mep-c', 'expected_revision': snap['revision']})
        self.assertEqual(saved['url'], 'https://factory-c.example.org/cse')
        invalid = {**settings, 'factory_c': {'mn_url': 'https://example.org/cse?query=1'}}
        with self.assertRaises(ValueError):
            factory_c_address.presets(invalid)


if __name__ == '__main__':
    unittest.main()
