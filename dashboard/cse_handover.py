"""CSE handover through the MEP's MEC 033 IoT API.

config.json (optional):
  "handover": {"iot_urls": {"A": "<MEP>/iots/v1/registered_iot_platforms", "B": "...", "C": "..."},
               "interval": 5, "verify_tls": true}

While a site's CSE does not answer, the server asks that site's MEP for the current CSE
(customServicesTransportInfo whose id is a CSE-ID: endpoint.uris[0] + "/" + name, with the scheme and
host of the iot URL, because the MEP reports an internal host). If the MEP reports another CSE, the new
address is saved. Everything that resolves the site address through resolve() follows it: judgement,
detection, the GET collector and demo commands. No answer from the MEP means the network is down; the
same CSE means it is restarting. In both cases nothing changes.
"""
import json
import ssl
import threading
import uuid
from urllib.error import HTTPError
from urllib.parse import urlsplit, urlunsplit
from urllib.request import HTTPSHandler, ProxyHandler, Request, build_opener

from core import NoRedirect, utc_now

KEY = 'cse_handover'
SITES = ('A', 'B', 'C')


def settings(cfg):
    value = cfg.get('handover') or {}
    if not isinstance(value, dict):
        raise ValueError('handover must be an object.')
    urls = value.get('iot_urls') or {}
    if not isinstance(urls, dict) or any(site not in SITES for site in urls):
        raise ValueError('handover.iot_urls must map A, B or C to a URL.')
    for site, url in urls.items():
        u = urlsplit(url) if isinstance(url, str) else None
        if not u or u.scheme not in ('http', 'https') or not u.hostname or u.username or u.password or u.fragment:
            raise ValueError(f'handover.iot_urls.{site} must be an HTTP(S) URL without credentials.')
    interval = value.get('interval', 5)
    if type(interval) not in (int, float) or not 1 <= interval <= 300:
        raise ValueError('handover.interval must be 1..300 seconds.')
    if type(value.get('verify_tls', True)) is not bool:
        raise ValueError('handover.verify_tls must be true or false.')
    return {'iot_urls': {k: v.rstrip('/') for k, v in urls.items()}, 'interval': interval,
            'verify_tls': value.get('verify_tls', True)}


def resolve(cfg, store, site, url):
    """Configured base URL -> (current URL, revision tag). A saved move only applies to the URL it replaced."""
    url = url.rstrip('/')
    item = (store.setting(KEY, {}) or {}).get(site)
    if isinstance(item, dict) and item.get('from') == url and isinstance(item.get('url'), str):
        return item['url'], str(item.get('revision', ''))
    return url, ''


def opener(verify_tls):
    context = ssl.create_default_context()
    if not verify_tls:
        context.check_hostname = False
        context.verify_mode = ssl.CERT_NONE
    return build_opener(ProxyHandler({}), NoRedirect(), HTTPSHandler(context=context))


def mep_cse(iot_url, verify_tls, timeout=3):
    """MEC 033 registered_iot_platforms -> (CSE-ID, CSEBase URL). Raises if the MEP does not answer."""
    with opener(verify_tls).open(Request(iot_url, headers={'Accept': 'application/json'}), timeout=timeout) as response:
        platforms = json.loads(response.read(1048577))
    q = urlsplit(iot_url)
    for platform in platforms if isinstance(platforms, list) else []:
        if not isinstance(platform, dict) or platform.get('enabled') is False:
            continue
        for info in platform.get('customServicesTransportInfo') or []:
            if not isinstance(info, dict):
                continue
            cse_id, name = str(info.get('id') or ''), info.get('name')
            uris = (info.get('endpoint') or {}).get('uris') or []
            if cse_id.startswith('/') and isinstance(name, str) and name and uris and isinstance(uris[0], str):
                u = urlsplit(uris[0].rstrip('/'))._replace(scheme=q.scheme, netloc=q.netloc)
                return cse_id, urlunsplit(u) + '/' + name
    raise ValueError('No CSE in the MEC 033 response (customServicesTransportInfo, id=/<CSE-ID>).')


class Handover:
    def __init__(self, cfg, store, bases):
        """bases: site -> callable returning the configured CSEBase URL, or None when handover does not apply."""
        self.cfg, self.store, self.bases = cfg, store, bases
        self.settings = settings(cfg)
        self.lock = threading.Lock()
        self.stopping = threading.Event()
        self.status = {site: {'state': 'idle'} for site in self.settings['iot_urls']}

    def start(self):
        if self.settings['iot_urls']:
            threading.Thread(target=self.run, name='cse-handover', daemon=True).start()

    def run(self):
        while not self.stopping.wait(self.settings['interval']):
            for site in self.settings['iot_urls']:
                try:
                    self.check(site)
                except Exception as exc:  # keep watching the other sites
                    self.status[site] = {'state': 'error', 'message': str(exc)[:300], 'checked_at': utc_now()}

    def alive(self, url):
        request = Request(url, headers={'Accept': 'application/json', 'X-M2M-Origin': self.cfg['origin'],
                                        'X-M2M-RVI': self.cfg['release'], 'X-M2M-RI': 'edgeway-ho-' + uuid.uuid4().hex})
        try:
            with opener(self.settings['verify_tls']).open(request, timeout=3):
                return True
        except HTTPError as exc:
            return exc.code < 500                     # the CSE answered; 5xx = gateway without a CSE behind it
        except (OSError, ValueError):
            return False

    def check(self, site):
        base = self.bases.get(site, lambda: None)()
        if not base:
            self.status[site] = {'state': 'idle', 'checked_at': utc_now()}
            return
        current, _ = resolve(self.cfg, self.store, site, base)
        if self.alive(current):
            self.status[site] = {'state': 'up', 'url': current, 'checked_at': utc_now()}
            return
        try:
            cse_id, url = mep_cse(self.settings['iot_urls'][site], self.settings['verify_tls'])
        except (OSError, ValueError) as exc:
            self.status[site] = {'state': 'network_down', 'url': current, 'message': str(exc)[:300], 'checked_at': utc_now()}
            return
        if url == current:
            self.status[site] = {'state': 'cse_restarting', 'url': current, 'cse_id': cse_id, 'checked_at': utc_now()}
            return
        with self.lock:
            data = dict(self.store.setting(KEY, {}) or {})
            if url == base.rstrip('/'):
                data.pop(site, None)                  # back to the configured CSE
            else:
                data[site] = {'from': base.rstrip('/'), 'url': url, 'cse_id': cse_id, 'previous': current,
                              'revision': uuid.uuid4().hex[:12], 'at': utc_now()}
            self.store.put_setting(KEY, data)
        self.status[site] = {'state': 'moved', 'url': url, 'previous': current, 'cse_id': cse_id, 'checked_at': utc_now()}

    def snapshot(self):
        return {'sites': self.status, 'moves': self.store.setting(KEY, {}) or {}}
