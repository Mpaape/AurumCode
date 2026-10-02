const fs = require("fs");
const path = require("path");
function readUpload(name) {
  return fs.readFileSync(path.join("/srv/uploads", name), "utf8");
}
module.exports = { readUpload };
