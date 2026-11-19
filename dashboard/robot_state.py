"""Interpret the operating phase while retaining the received state in records."""


def primary_state(value):
    if not isinstance(value, str):
        return None
    return value.split('/', 1)[0].strip() or None
