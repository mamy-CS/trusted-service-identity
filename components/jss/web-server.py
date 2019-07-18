from flask import Flask, request
import os
from os.path import join, exists
import subprocess

app = Flask(__name__)

if __name__ == '__main__':
    app.run(debug=True,host='0.0.0.0')

@app.route('/')
def index():
    return "JSS server"

@app.route('/getJWT')
def get():
    args = request.args.copy()
    claims = ""
    if args:
        claims = "--claims="
        for k in args:
            claims = claims + k + ":" + args[k] + "|"
    statedir = os.getenv('STATEDIR') or '/tmp'
    privkeyfile = join(statedir, "private.key")
    try:
        out = subprocess.check_output(['/usr/local/bin/gen-jwt.py',privkeyfile,'--iss','example-issuer', claims])
        return str(out)
    except Exception as e:
        print e.output
        return ("Error: %s" % e.output), 503

@app.route('/public/getCSR')
def getCSR():
    statedir = os.getenv('STATEDIR') or '/tmp'
    csrfile = join(statedir,"server.csr")
    with open(csrfile) as f:
        csr = f.read().strip()
        return str(csr)

@app.route('/public/postX5c', methods=["POST"])
def postX5c():
    try:
        statedir = os.getenv('STATEDIR') or '/tmp'
        x5cfile = join(statedir, "x5c")
        # if file already exists, don't all to override it
        if exists(x5cfile):
            # return 403 Forbidden, 406 Not Accesptable or 409 Conflict
            return "File already exists.", 403
        if request.data and len(request.data) > 0:
            with open(x5cfile, "w+") as f:
                f.write(request.data)
                f.close()
                return "Upload of x5c successful"
    except Exception as e:
        print (e)
        #flash(e)
        return ("Error %s" % e), 500
