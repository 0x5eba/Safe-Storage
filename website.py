import os, time
from flask import Flask, render_template, request
app = Flask(__name__)
from datetime import datetime

path = os.getcwd() + "/Blockchain"

@app.route('/uploader', methods = ['GET', 'POST'])
def upload_file():
    if request.method == 'POST':
        f = request.files['file']
        radio = request.form['optradio']
        new_name = "documents/" + f.filename + str(time.time())
        f.save(new_name)
        if radio == "upload":
            os.system("cd " + path + "; go run main.go c 1 " + "../" + new_name)
            time.sleep(0.5)
            # os.remove(new_name)
            with open(path + "/tmp") as f:
                line = f.readline()
                if line == "yes":
                    return 'Successfully uploaded'
                if line == "no":
                    return 'Not successfully uploaded'

        if radio == "check":
            os.system("cd " + path + "; go run main.go c 2 " + "../" + new_name)
            time.sleep(0.5)
            # os.remove(new_name)
            with open(path + "/tmp") as f:
                line = f.readline()
                if line != "no":
                    return 'This file exist in the blockchain with timestamp ' + str(datetime.fromtimestamp(int(line)))
                if line == "no":
                    return "This file doesn't exist in the blockchain"
        
		
if __name__ == '__main__':
   app.run(debug = True)
