"""Readable event presentation derived only from fields actually received."""
import json
import re
from robot_state import primary_state

STATES = {'anomaly_detected':'Suspected robot fault', 'inspection_wait':'Robot fault', 'cross_check':'Cross-checking', 'idle':'Idle', 'loading':'Picking up', 'to_drop':'Moving to drop-off', 'unloading':'Unloading', 'done':'Task complete', 'moving':'Moving', 'navigating':'Moving',
          'arrived':'Arrived at destination', 'completed':'Task complete', 'stopped':'Stopped',
          'error':'Error', 'fault':'Robot fault', 'failed':'Failed', 'paused':'Paused',
          'waiting':'Idle', 'blocked':'Path blocked', 'collision':'Collision',
          'carrying':'Carrying', 'loaded':'Loaded', 'pickup':'Pickup', 'picking':'Pickup',
          'dropoff':'Drop-off', 'dropping':'Drop-off',
          'reset':'Resetting', 'to_pick':'Moving to pickup', 'aborted':'Task aborted'}

EVENTS = {'collision':'Collision detected', 'robot_fault':'Robot fault detected', 'robot_error':'Robot error detected',
          'object_pickup':'Pickup', 'pickup':'Pickup',
          'object_dropoff':'Drop-off', 'dropoff':'Drop-off',
          'object_carried':'Carrying point', 'obstacle_wait':'Waiting point'}


def text(value, limit=180):
    if not isinstance(value, str):
        return ''
    value = ' '.join(value.split())
    return value if len(value) <= limit else value[:limit - 1] + '…'


LEGACY_STATES = {
    '대기':'Idle', '이동':'Moving', '이동 중':'Moving', '목적지 도착':'Arrived at destination',
    '작업 완료':'Task complete', '정지':'Stopped', '오류':'Error', '로봇 이상':'Robot fault',
    '실패':'Failed', '작업 실패':'Failed', '일시 정지':'Paused', '통행 불가':'Path blocked',
    '경로 대기':'Path blocked', '충돌':'Collision',
    '운반':'Carrying', '운반 중':'Carrying', '적재':'Loaded', '적재 중':'Loaded', '물건 집기':'Pickup',
    '물건 내려놓기':'Drop-off',
}
LEGACY_EVENTS = {
    '충돌 감지':'Collision detected', '로봇 이상 감지':'Robot fault detected',
    '로봇 오류 감지':'Robot error detected', '물건 집기':'Pickup',
    '물건 내려놓기':'Drop-off', '운반 지점':'Carrying point', '대기 지점':'Waiting point',
    '통행 제한 감지':'Traffic restriction detected', '구동부 이상 감지':'Drive fault detected',
    '판단 유보':'Assessment deferred', '충돌 발생':'Collision detected',
    '로봇 이상 발생':'Robot fault detected', '로봇 오류 발생':'Robot error detected',
}


def display_target(value):
    """Translate only the importer-generated name; preserve custom names and IDs."""
    match = re.fullmatch(r'공장 ([ABC]) · 목데이터 조회', value)
    return f'Factory {match[1]} · Mock Data' if match else value


def present(content, robot_name):
    result = _present(content, robot_name)
    result['target'] = display_target(result['target'])
    title = result['title']
    state = re.fullmatch(r'(?:상태|Status) · (.+)', title)
    if state and state[1] in LEGACY_STATES:
        result['title'] = 'Status · ' + LEGACY_STATES[state[1]]
    else:
        result['title'] = LEGACY_EVENTS.get(title, title)
    return result


def _present(content, robot_name):
    if isinstance(content, str):
        try:
            content = json.loads(content)
        except (ValueError, RecursionError):
            pass
    result = {'title':'Data received', 'target':text(robot_name, 80), 'kind':'Other',
              'level':'info', 'context':{}, 'restriction':'', 'note':'', 'test':False, 'description':''}
    if isinstance(content, str):
        result.update(title=text(content) or '(Empty message)', kind='Log')
        return result
    if not isinstance(content, dict):
        result['title'] = 'Array data received' if isinstance(content, list) else 'Data received'
        return result
    if type(content.get('disconnect_duration_ms')) in (int, float):
        # IPE outage record (<robot>/log), posted once the robot is back online and its backlog is sent.
        seconds = content['disconnect_duration_ms'] / 1000
        lost, held = content.get('data_loss'), content.get('buffered_count')
        result.update(title=f'Network disconnected · {seconds:.1f} s', kind='Log',
                      level='error' if type(lost) is int and lost > 0 else 'warning',
                      description=f'The robot lost its connection to the CSE for {seconds:.1f} s and has reconnected '
                                  f'(this record is from the reconnection). Buffered {held}, lost {lost}.')
        return result
    event = content.get('event')
    event = event if isinstance(event, dict) else {'message':event}
    context = content.get('context')
    context = context if isinstance(context, dict) else {}
    message = text(event.get('message')) or text(content.get('message')) or text(content.get('title'))
    result['description'] = text(event.get('description'), 1000) or text(content.get('description'), 1000)
    state = text(content.get('state'), 80) or text(content.get('nav'), 80)
    phase = primary_state(state)
    # Compact metadata survives truncated sensor previews; original content is unchanged.
    result['reported_state'] = state or None
    stamp = content.get('t_wall')
    result['state_time'] = stamp if type(stamp) in (int, float) and -1e15 < stamp < 1e15 else None
    result['state_priority'] = 2 if text(content.get('state'), 80) else 1 if state else 0
    event_type = text(event.get('type')) or text(content.get('event_type'))
    if event_type == 'object_interaction':
        event_type = text(event.get('action'))
    result['event_type'] = event_type or None
    result['target'] = text(event.get('target'), 80) or text(content.get('target'), 80) or result['target']
    level = event.get('level', content.get('level'))
    result['level'] = level if level in ('info','warning','error','success') else 'info'
    result['test'] = content.get('test_data') is True or bool(text(content.get('test_id')))
    if message:
        result.update(title=message, kind='Event')
    elif event_type in EVENTS:
        result.update(title=EVENTS[event_type], kind='Event')
    elif phase:
        result.update(title='Status · ' + STATES.get(phase, phase), kind='Status')
    elif any(key in content for key in ('pose','imu','enc','load_kg','t_sim')):
        times = content.get('t_sim')
        count = len(times) if isinstance(times, list) else 0
        result.update(title='Sensor data received' + (f' · {count} samples' if count else ''), kind='Sensor')
    else:
        result['title'] = 'Structured data received'
    if level not in ('info','warning','error','success'):
        if event_type in ('collision','robot_fault','robot_error') or phase in ('inspection_wait','error','fault','failed','collision'):
            result['level'] = 'error'
    if phase == 'anomaly_detected' and result['level'] == 'info':
        result['level'] = 'warning'
    result['context'] = {key:text(context.get(key), 2000) for key in
                         ('inspection_target','reason','command','result') if text(context.get(key), 2000)}
    result['restriction'] = text(context.get('restriction'), 60)
    result['note'] = text(context.get('note'), 1000)
    return result
