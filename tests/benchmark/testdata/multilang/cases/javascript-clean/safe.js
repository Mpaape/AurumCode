function findUser(db, name) {
  return db.query("SELECT id FROM users WHERE name = ?", [name]);
}
module.exports = { findUser };
