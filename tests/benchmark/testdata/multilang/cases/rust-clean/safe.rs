use std::process::Command;

pub fn archive(name: &str) -> std::io::Result<()> {
    Command::new("tar").args(["czf", "out.tgz", name]).status()?;
    Ok(())
}
