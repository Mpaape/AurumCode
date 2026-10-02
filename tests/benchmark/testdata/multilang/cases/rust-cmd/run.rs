use std::process::Command;

pub fn archive(name: &str) -> std::io::Result<()> {
    Command::new("sh").arg("-c").arg(format!("tar czf out.tgz {}", name)).status()?;
    Ok(())
}
