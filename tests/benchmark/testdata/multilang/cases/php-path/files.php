<?php
function read_upload($name) {
    return file_get_contents("/srv/uploads/" . $name);
}
