"""GET latest status/telemetry (and the IPE outage log) for explicit Isaac robot IDs; never writes to the CSE.

This is latest-value polling, not a lossless event subscription. The original CIN
and retrieval provenance are retained. Existing mock polling remains separate.
"""
import argparse
import cse_handover
import factory_c_address
from concurrent.futures import FIRST_COMPLETED, ThreadPoolExecutor, wait
from functools import partial
import json
import re
import signal
import sqlite3
import ssl
import threading
import time
import uuid
from urllib.error import HTTPError
from urllib.parse import urlsplit
from urllib.request import HTTPSHandler, ProxyHandler, Request, build_opener

from core import NoRedirect, Store, dumps, load_config, strict_json, utc_now


def fetch(opener, url, timeout=5, *, origin, release):
    request_id = "edgeway-poll-" + uuid.uuid4().hex
    request = Request(url, method="GET", headers={"Accept": "application/json",
        "X-M2M-Origin": origin, "X-M2M-RVI": release, "X-M2M-RI": request_id})
    with opener.open(request, timeout=timeout) as response:
        if response.status != 200 or response.headers.get("X-M2M-RSC") != "2000":
            raise ValueError("Expected HTTP 200 / oneM2M RSC 2000")
        if response.headers.get("X-M2M-RI") != request_id:
            raise ValueError("Response request ID mismatch")
        raw = response.read(1024 * 1024 + 1)
    if len(raw) > 1024 * 1024:
        raise ValueError("Response exceeds 1 MiB")
    return raw



def save_cin(store, cfg, base, csi, robot_id, channel, payload, tls_verified):
    cin = payload.get('m2m:cin') if isinstance(payload, dict) else None
    if not isinstance(cin, dict) or cin.get('ty') != 4:
        raise ValueError('Expected m2m:cin ty=4')
    for key in ('ri', 'pi', 'ct'):
        if not isinstance(cin.get(key), str) or not 1 <= len(cin[key]) <= 256:
            raise ValueError('Missing or invalid CIN '+key)
    content = strict_json(cin.get('con')) if isinstance(cin.get('con'), str) else cin.get('con')
    if not isinstance(content, dict) or content.get('robot') != robot_id:
        raise ValueError('CIN robot does not match requested AE: '+robot_id)
    if channel not in ('status', 'telemetry', 'log'):
        raise ValueError('Unsupported channel')
    if channel == 'log':                      # IPE outage record, posted once after the backlog is sent
        if type(content.get('disconnect_duration_ms')) not in (int, float):
            raise ValueError('Expected disconnect_duration_ms')
    elif not isinstance(content.get('pose'), dict) or 't_sim' not in content:
        raise ValueError('Expected pose and t_sim')
    source = {**cfg, 'cse_url':base, 'cse_id':csi}
    robot = {'id':robot_id, 'name':robot_id, 'ae':robot_id, 'container':channel}
    raw = {'m2m:cin':cin, '_edgeway_retrieval':{
        'mode':'isaac_latest_poll', 'source_url':base+'/'+robot_id+'/'+channel+'/la',
        'imported_at':utc_now(), 'tls_verified':tls_verified,
        'note':'Latest-value GET polling; intermediate CINs may be missed'}}
    event_id, created = store.save(source, robot, cin, '', dumps(raw))
    return {'robot':robot_id, 'channel':channel, 'event_id':event_id, 'created':created}


def poll_targets(targets, retrieve, consume, report_error, interval, workers, stop, once=False):
    """Independent start-to-start cadence; bounded workers and one request per CNT.

    Network I/O is parallel, but completed results are saved on this thread.
    A slow/failed endpoint does not create a whole-round barrier or a catch-up queue.
    """
    due = {target: time.monotonic() for target in targets}
    pending, active, completed = {}, set(), set()
    failed = False
    with ThreadPoolExecutor(max_workers=workers, thread_name_prefix='isaac-get') as pool:
        while not stop.is_set():
            now = time.monotonic()
            ready = sorted((target for target in targets if target not in active
                            and (not once or target not in completed) and due[target] <= now),
                           key=lambda target: due[target])
            for target in ready[:max(0, workers-len(pending))]:
                started = time.monotonic()
                pending[pool.submit(retrieve, *target)] = (target, started)
                active.add(target)
            if once and len(completed) == len(targets):
                break
            # Wake when a result completes, a free worker's next target is due,
            # or shutdown is requested. Never queue work behind busy workers.
            delay = 0.25
            if len(pending) < workers:
                deadlines = [due[t] for t in targets if t not in active
                             and (not once or t not in completed)]
                if deadlines:
                    delay = min(delay, max(0, min(deadlines)-time.monotonic()))
            if not pending:
                stop.wait(delay)
                continue
            done, _ = wait(pending, timeout=delay, return_when=FIRST_COMPLETED)
            for future in done:
                target, started = pending.pop(future)
                active.remove(target)
                completed.add(target)
                if stop.is_set():
                    continue
                try:
                    consume(*target, future.result())
                except (ValueError, OSError, sqlite3.Error) as exc:
                    failed = True
                    report_error(*target, exc)
                # Overruns skip missed ticks; no bursts, no extra interval sleep.
                due[target] = max(started + interval, time.monotonic())
    return int(failed) if once else 0


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument('--config', default='config.json')
    ap.add_argument('--site', choices=('A','B','C'), required=True)
    ap.add_argument('--robots', nargs='+', required=True, help='Exact AE IDs; must belong to --site')
    ap.add_argument('--base-url', help='Override the configured CSEBase URL for factories A and B')
    ap.add_argument('--interval', type=float, default=1, help='Target seconds between request starts per CNT (0.25..60)')
    ap.add_argument('--workers', type=int, default=4, help='Maximum concurrent GET requests (1..8)')
    ap.add_argument('--timeout', type=float, default=5, help='Per-request socket timeout in seconds (0.25..30)')
    ap.add_argument('--once', action='store_true')
    tls = ap.add_mutually_exclusive_group()
    tls.add_argument('--ca-file')
    tls.add_argument('--insecure-demo', action='store_true', help='Skip certificate verification for this GET-only worker')
    args = ap.parse_args()
    if not 0.25 <= args.interval <= 60:
        ap.error('interval must be between 0.25 and 60 seconds')
    if not 1 <= args.workers <= 8:
        ap.error('workers must be between 1 and 8')
    if not 0.25 <= args.timeout <= 30:
        ap.error('timeout must be between 0.25 and 30 seconds')
    if len(args.robots)>32 or len(set(args.robots))!=len(args.robots) or any(
        not re.fullmatch(r'site'+args.site+r'_AMR_[1-9][0-9]*', r) for r in args.robots):
        ap.error('Provide 1..32 distinct site'+args.site+'_AMR_<number> IDs')
    cfg = load_config(args.config)
    urls = cfg.get('collector_cse_urls', {})
    if not isinstance(urls, dict):
        ap.error('collector_cse_urls must be an object')
    selected_base = args.base_url or urls.get(args.site, cfg['cse_url'])
    if not isinstance(selected_base, str):
        ap.error('The collector CSEBase URL must be a string')
    base = selected_base.rstrip('/')
    u = urlsplit(base)
    if u.scheme not in ('http','https') or not u.hostname or u.username or u.password or u.query or u.fragment:
        ap.error('base-url must be an HTTP or HTTPS CSEBase URL without credentials/query')
    read = partial(fetch, origin=cfg['origin'], release=cfg['release'])
    context = ssl.create_default_context(cafile=args.ca_file)
    if args.insecure_demo:
        context.check_hostname = False
        context.verify_mode = ssl.CERT_NONE
        print('TLS verification disabled for this GET-only demo worker.', flush=True)
    opener = build_opener(ProxyHandler({}), NoRedirect(), HTTPSHandler(context=context))
    stop = threading.Event()
    signal.signal(signal.SIGINT, lambda *_: stop.set())
    signal.signal(signal.SIGTERM, lambda *_: stop.set())
    thread_state = threading.local()

    def retrieve(robot, channel):
        try:
            return retrieve_latest(robot, channel)
        except HTTPError as exc:
            if channel == 'log' and exc.code == 404:
                return None               # no outage recorded yet (empty log container)
            raise

    def retrieve_latest(robot, channel):
        # urllib handlers are private to each worker; TLS and redirects retain
        # the same policy as the original collector.
        if not hasattr(thread_state, 'opener'):
            thread_state.opener = build_opener(ProxyHandler({}), NoRedirect(), HTTPSHandler(context=context))
        if args.site == 'C':
            if not hasattr(thread_state, 'cse_cache'):
                thread_state.cse_cache = {}
            return factory_c_address.retrieve_current(cfg, store, thread_state.opener, read,
                strict_json, robot, channel, args.timeout, thread_state.cse_cache)
        # A/B: follow a CSE move saved by the dashboard (cse_handover); CSE-ID cached per address.
        current, _ = cse_handover.resolve(cfg, store, args.site, base)
        if not hasattr(thread_state, 'csi_cache'):
            thread_state.csi_cache = {}
        if current not in thread_state.csi_cache:
            cb = strict_json(read(thread_state.opener, current, args.timeout)).get('m2m:cb', {})
            if cb.get('ty') != 5 or not isinstance(cb.get('csi'), str) or not cb['csi'].startswith('/'):
                raise ValueError('Expected a CSEBase with csi')
            thread_state.csi_cache = {current: cb['csi']}
        return current, thread_state.csi_cache[current], strict_json(
            read(thread_state.opener, current+'/'+robot+'/'+channel+'/la', args.timeout))

    def consume(robot, channel, payload):
        if payload is None:
            return
        if args.site == 'C':
            result = factory_c_address.consume_current(cfg, store, payload,
                lambda source, remote_csi, content: save_cin(store, cfg, source, remote_csi,
                    robot, channel, content, urlsplit(source).scheme == 'https' and not args.insecure_demo))
            if result is None:
                return
        else:
            current, current_csi, content = payload
            result = save_cin(store, cfg, current, current_csi, robot, channel, content,
                              urlsplit(current).scheme == 'https' and not args.insecure_demo)
        if result['created']:
            print(json.dumps(result), flush=True)

    def report_error(robot, channel, exc):
        print(json.dumps({'robot': robot, 'channel': channel, 'error': str(exc)}), flush=True)

    store = None
    try:
        store = Store(cfg['database'])
        if args.site != 'C':
            while not stop.is_set():
                try:
                    current, _ = cse_handover.resolve(cfg, store, args.site, base)
                    cb = strict_json(read(opener, current, args.timeout)).get('m2m:cb', {})
                    csi = cb.get('csi')
                    if cb.get('ty')!=5 or not isinstance(csi,str) or not csi.startswith('/'):
                        raise ValueError('Expected a CSEBase with csi')
                    break
                except (ValueError, OSError) as exc:
                    if args.once:
                        raise
                    print(f'WAITING_FOR_CSE site={args.site} base={current}: {exc}; retry in 2s', flush=True)
                    stop.wait(2)
            if stop.is_set():
                return 0
        else:
            print('Factory C uses the saved dashboard address; --base-url is not used for C.', flush=True)
        print(f'Latest polling: workers={args.workers}, target period={args.interval:g}s/CNT, '
              f'timeout={args.timeout:g}s; GET only. Ctrl+C stops only this collector.', flush=True)
        targets = [(robot, channel) for channel in ('telemetry', 'status', 'log') for robot in args.robots]
        return poll_targets(targets, retrieve, consume, report_error, args.interval,
                            args.workers, stop, args.once)
    except KeyboardInterrupt:
        print('Isaac polling stopped.',flush=True)
        return 0
    except (ValueError, OSError, sqlite3.Error) as exc:
        print('Polling failed: '+str(exc),flush=True)
        return 1


if __name__ == '__main__':
    raise SystemExit(main())
