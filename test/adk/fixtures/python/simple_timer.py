import gapi
import time

def start(stop_evt=None):
    gapi.log.info("Timer agent started")
    
    # Simple timer loop
    import time
    if stop_evt:
        while not stop_evt.is_set():
            gapi.log.info("Tick")
            time.sleep(1)
    else:
        while True:
            gapi.log.info("Tick")
            time.sleep(1)
