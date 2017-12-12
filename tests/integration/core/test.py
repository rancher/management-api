from common_fixtures import *  # NOQA


def test_cluster(client):
    client.list_cluster()
    name = random_str()
    c = client.create_cluster(name=name)
    assert c.state == 'initializing'
    c = client.wait_success(c)
    assert c.state == 'active'
