use std::fs;

pub fn read_upload(name: &str) -> std::io::Result<String> {
    fs::read_to_string(format!("/srv/uploads/{}", name))
}
