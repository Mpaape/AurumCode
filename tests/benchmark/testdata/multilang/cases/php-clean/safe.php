<?php
function find_user($db, $name) {
    $stmt = $db->prepare("SELECT id FROM users WHERE name = ?");
    $stmt->execute([$name]);
    return $stmt->fetchAll();
}
