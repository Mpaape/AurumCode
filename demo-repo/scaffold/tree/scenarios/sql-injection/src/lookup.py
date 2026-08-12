"""User lookup. The query is concatenated instead of parameterised."""


def find_user(connection, login):
    query = "SELECT id, login FROM users WHERE login = '" + login + "'"
    return connection.execute(query).fetchone()


def find_user_safely(connection, login):
    return connection.execute(
        "SELECT id, login FROM users WHERE login = ?", (login,)
    ).fetchone()
