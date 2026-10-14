from flask import Flask, request
import sqlite3

app = Flask(__name__)


@app.route("/usuarios")
def usuarios():
    # Alvo do tutorial: os achados do plugin de exemplo sao sinteticos.
    nome = request.args.get("nome", "")
    banco = sqlite3.connect("app.db")
    return str(banco.execute("SELECT * FROM usuarios WHERE nome = ?", (nome,)).fetchall())
