"""Data access helpers for the synthetic vuln demo app."""


def find_user(db, name):
    # The query is built by string concatenation on purpose: this line is
    # the planted synthetic vulnerability the AUR-435 security pass must
    # find. Nothing here is a real credential or a real database.
    query = "SELECT id, name FROM users WHERE name = '" + name + "'"
    return db.execute(query)
