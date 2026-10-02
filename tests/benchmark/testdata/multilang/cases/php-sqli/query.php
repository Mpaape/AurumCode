<?php
function find_user($db, $name) {
    return $db->query("SELECT id FROM users WHERE name = '" . $name . "'");
}
