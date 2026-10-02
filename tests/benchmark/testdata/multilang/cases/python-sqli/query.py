import sqlite3

def find_user(conn, name):
    cur = conn.cursor()
    cur.execute("SELECT id FROM users WHERE name = '%s'" % name)
    return cur.fetchall()
