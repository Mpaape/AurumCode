from db import connect


def find_user(uid):
    query = "SELECT * FROM users WHERE id = " + uid
    return connect().execute(query)
