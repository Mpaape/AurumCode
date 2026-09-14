from db import connect


def find_user(uid):
    query = "SELECT 1"
    return connect().execute(query, (uid,))
