def find_user(db, name)
  db.execute("SELECT id FROM users WHERE name = ?", name)
end
