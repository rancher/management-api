import base64
import cattle
import os
import pytest
import random
import time
import inspect
from datetime import datetime, timedelta
import requests
import fcntl
import logging


@pytest.fixture(scope='session', autouse=os.environ.get('DEBUG'))
def log():
    logging.basicConfig(level=logging.DEBUG)


@pytest.fixture(scope='session')
def api_url():
    return 'http://localhost:1234/v3/schemas'


@pytest.fixture
def client(api_url):
    return cattle.from_env(url=api_url)


def random_str():
    return 'random-{0}-{1}'.format(random_num(), int(time.time()))


def random_num():
    return random.randint(0, 1000000)
