"""Shared C address selection for the dashboard and its GET collector."""
import json
from pathlib import Path
import threading
from urllib.parse import urlsplit
import uuid

KEY = 'factory_c_address'
ADDRESS_LOCK = threading.RLock()


def presets(cfg):
    settings = cfg.get('factory_c', {})
    if not isinstance(settings, dict):
        raise ValueError('factory_c must be an object.')
    choices = {}
    for key, label, url in (
        ('local', 'Local test', settings.get('local_url', 'http://127.0.0.1:8081/SiteB')),
        ('mep-c', 'Factory C MN-CSE', settings.get('mn_url')),
    ):
        if key == 'mep-c' and url is None:
            continue
        if not isinstance(url, str) or any(ch.isspace() for ch in url):
            raise ValueError('A Factory C CSEBase URL must be a string without whitespace.')
        parsed = urlsplit(url)
        if parsed.scheme not in ('http', 'https') or not parsed.hostname or parsed.username or parsed.password or parsed.query or parsed.fragment:
            raise ValueError('Use an HTTP(S) CSEBase URL without credentials, queries or fragments.')
        choices[key] = {'label': label, 'url': url.rstrip('/')}
    return choices


def selection(cfg, store):
    choices = presets(cfg)
    value = store.setting(KEY)
    if value is not None:
        if not isinstance(value, dict) or not isinstance(value.get('preset'), str) or value['preset'] not in choices or not isinstance(value.get('revision'), str):
            raise ValueError('Invalid saved factory C address selection.')
        preset, revision = value['preset'], value['revision']
    else:
        config = Path(cfg['config_path']).parent/'demo-command.json'
        try:
            data = json.loads(config.read_text(encoding='utf-8'))
        except FileNotFoundError:
            data = {}
        if not isinstance(data, dict) or not isinstance(data.get('sites', {}), dict):
            raise ValueError('Invalid demo-command.json sites settings.')
        preset = cfg.get('factory_c', {}).get('initial_preset')
        if preset is None:
            # Preserve the previous initial selection for deployments with a matching command URL.
            url = data.get('sites', {}).get('C', choices['local']['url'])
            if not isinstance(url, str):
                raise ValueError('Factory C initial address must be a URL.')
            url = url.rstrip('/')
            mn = choices.get('mep-c')
            if mn and url == mn['url'].replace('https://', 'http://', 1):
                url = mn['url']
            preset = next((key for key, item in choices.items() if item['url'] == url), None)
        if not isinstance(preset, str) or preset not in choices:
            raise ValueError('Choose a configured Factory C preset in config.json.')
        revision = 'initial-'+preset
    return {'preset': preset, 'revision': revision, **choices[preset]}


def snapshot(cfg, store):
    chosen = selection(cfg, store)
    return {**chosen, 'options': [{'id': key, **item} for key, item in presets(cfg).items()]}


def save(cfg, store, data):
    preset = data.get('preset')
    if not isinstance(preset, str) or preset not in presets(cfg):
        raise ValueError('Choose a configured Factory C preset.')
    with ADDRESS_LOCK:
        current = selection(cfg, store)
        if data.get('expected_revision') != current['revision']:
            raise ValueError('Address changed in another window. Reload and choose again.')
        if current['preset'] != preset:
            store.put_setting(KEY, {'preset': preset, 'revision': uuid.uuid4().hex})
        return snapshot(cfg, store)


def retrieve_current(cfg, store, opener, fetch, decode, robot, channel, timeout, cache):
    """Carry provenance with each result, even if selection changes during GET."""
    chosen = selection(cfg, store)
    base = chosen['url']
    token = (base, chosen['revision'])
    if token not in cache:
        cb = decode(fetch(opener, base, timeout)).get('m2m:cb', {})
        csi = cb.get('csi')
        if cb.get('ty') != 5 or not isinstance(csi, str) or not csi.startswith('/'):
            raise ValueError('Expected a CSEBase with csi')
        cache.clear()
        cache[token] = csi
    payload = decode(fetch(opener, base+'/'+robot+'/'+channel+'/la', timeout))
    return chosen, cache[token], payload


def consume_current(cfg, store, result, consume):
    chosen, csi, payload = result
    current = selection(cfg, store)
    if (current['revision'], current['url']) != (chosen['revision'], chosen['url']):
        return None  # Discard old-target responses still in flight at a switch.
    return consume(chosen['url'], csi, payload)
