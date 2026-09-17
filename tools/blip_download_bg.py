import time, os
os.environ['HF_HUB_DISABLE_PROGRESS_BARS']='1'
os.environ['HF_HUB_DISABLE_SYMLINKS_WARNING']='1'
from huggingface_hub import snapshot_download
log = r'C:\Zorion2\tools\blip_download.log'
for attempt in range(1, 30):
    try:
        with open(log, 'a') as f: f.write('attempt %d\n' % attempt)
        p = snapshot_download('Salesforce/blip-image-captioning-base')
        with open(log, 'a') as f: f.write('DONE: %s\n' % p)
        break
    except Exception as e:
        with open(log, 'a') as f: f.write('err: %s\n' % e)
        time.sleep(5)
