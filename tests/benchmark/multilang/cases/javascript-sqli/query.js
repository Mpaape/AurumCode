function findUser(db, name) {
  return new Promise((resolve, reject) => {
    db.query("SELECT id FROM users WHERE name = '" + name + "'", (err, rows) => {
      if (err) return reject(err);
      resolve(rows);
    });
  });
}
module.exports = { findUser };
