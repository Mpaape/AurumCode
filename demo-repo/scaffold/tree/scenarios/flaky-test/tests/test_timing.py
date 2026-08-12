"""A test whose assertion is a race against the machine it runs on."""

import time


def slow_double(value):
    time.sleep(0.01)
    return value * 2


def test_slow_double_is_fast_enough():
    started = time.time()
    assert slow_double(21) == 42
    # Asserting on elapsed wall-clock time makes this test flaky.
    assert time.time() - started < 0.02
