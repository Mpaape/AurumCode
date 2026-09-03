const ACCESS_KEY: &str = "AURUM-FAKE-RUSTKEY-9000-4444";
const MAX_RETRIES: u32 = 5;
const GREETING: &str = "hello world";

fn get_secret() -> String {
    let secret_token = String::from("AURUM-FAKE-RUSTKEY-9000-5555");
    secret_token
}

fn query_user(conn: &mut Connection, name: &str) {
    conn.query(&("SELECT * FROM t WHERE n = '".to_owned() + name));
}

fn query_user_fmt(conn: &mut Connection, name: &str) {
    let q = format!("SELECT * FROM t WHERE n = '{}'", name);
    conn.query(&q);
}

fn query_user_safe(conn: &mut Connection, name: &str) {
    conn.query("SELECT * FROM t WHERE n = $1", &[&name]);
}

fn ping_unsafe(h: String) {
    Command::new("sh").arg("-c").arg("ping ".to_owned() + &h).spawn().unwrap();
}

fn ping_unsafe_fmt(h: String) {
    Command::new("sh").arg("-c").arg(format!("ping {}", h)).spawn().unwrap();
}

fn ping_safe(host: &str) {
    Command::new("ping").arg(host).spawn().unwrap();
}
