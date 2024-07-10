from flask import Flask, request

app = Flask(__name__)


@app.route('/webhook', methods=['POST'])
def webhook():
    print("Headers: ", request.headers)
    print("Body: ", request.json)
    return '', 200


if __name__ == '__main__':
    app.run(host='0.0.0.0', port=50000)
