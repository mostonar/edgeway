"""Verify existing notification addresses without changing the data source identity."""
from urllib.parse import urlsplit

from core import CSEError, utc_now


def delivery_bases(cfg, profile):
    primary = profile['notification_base'].rstrip('/')
    bases = [primary]
    # A local test must remain on its selected local receiver.
    if urlsplit(primary).hostname in ('127.0.0.1', 'localhost', '::1'):
        return bases
    alternate = urlsplit(cfg.get('notification_url', ''))
    if (alternate.scheme in ('http', 'https') and alternate.hostname
            and not alternate.username and not alternate.password):
        base = alternate.scheme+'://'+alternate.netloc
        if base not in bases:
            bases.append(base)
    return bases


def callback_url(cfg, conn, route):
    bases = delivery_bases(cfg, conn['profile'])
    preferred = conn.get('callback_delivery_base')
    return (preferred if preferred in bases else bases[0])+route+conn['token']


def ensure_subscription(rpc, cfg, conn, parent, route):
    """Only retry an alternative address after an explicit CSE 5204 response.

    The same subscription name is reused. Timeouts have an unknown outcome and
    are left for the next GET/retry instead of creating another subscription.
    No container, data value, clear request, or profile key is changed here.
    """
    bases = delivery_bases(cfg, conn['profile'])
    preferred = conn.get('callback_delivery_base')
    if preferred in bases:
        bases.remove(preferred)
        bases.insert(0, preferred)
    candidates = {base: base+route+conn['token'] for base in bases}
    attempts = []
    conn['delivery_attempts'] = attempts
    try:
        existing = rpc(conn['profile'], 'GET', conn['sub_path']).get('m2m:sub')
        if not isinstance(existing, dict):
            raise ValueError('Invalid existing subscription response.')
    except CSEError as exc:
        if exc.status != 404:
            raise
        existing = None

    def matches(sub, callback):
        return (isinstance(sub, dict) and sub.get('ty') == 23
                and isinstance(sub.get('ri'), str) and bool(sub['ri'])
                and sub.get('nu') == [callback] and sub.get('nct') == 1
                and isinstance(sub.get('enc'), dict) and sub['enc'].get('net') == [3])

    def delivery(base):
        return {'callback': candidates[base], 'callback_delivery_base': base,
                'delivery_attempts': attempts}

    if existing is not None:
        if not isinstance(existing, dict) or existing.get('ty') != 23 or not existing.get('ri'):
            raise ValueError('Invalid existing subscription response.')
        for base, callback in candidates.items():
            if matches(existing, callback):
                return existing, delivery(base)

    for base, callback in candidates.items():
        desired = {'enc': {'net': [3]}, 'nct': 1, 'nu': [callback]}
        method, target = ('PUT', conn['sub_path']) if existing is not None else ('POST', parent)
        if method == 'POST':
            desired['rn'] = conn['sub_name']
        try:
            sub = rpc(conn['profile'], method, target, {'m2m:sub': desired},
                      23 if method == 'POST' else None).get('m2m:sub')
        except CSEError as exc:
            attempts.append({'base': base, 'at': utc_now(), 'http': exc.status, 'rsc': str(exc.rsc)})
            if str(exc.rsc) != '5204':
                raise
            if base == bases[-1]:
                raise CSEError('Subscription verification failed (5204) for '+', '.join(bases)+
                               '. GET reception continues; check callback arrival and CSE outbound routing.',
                               exc.status, exc.rsc) from exc
            continue
        if not matches(sub, callback):
            raise ValueError('Subscription response does not match the requested notification address/events.')
        attempts.append({'base': base, 'at': utc_now(), 'http': 201 if method == 'POST' else 200,
                         'rsc': '2001' if method == 'POST' else '2004'})
        return sub, delivery(base)
