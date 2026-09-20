# -*- coding: utf-8 -*-
# Батч-раннер иконок биомов (пачка 1): этап 1 (форма, ControlNet Canny) -> этап 2 (детализация, img2img).
# Выход: ai_drafts/biomes/batch_p1/<num>.png. Продолжение: --start N --count M. Лог: batch_p1/log.txt.
import argparse, json, os, shutil, time, urllib.request
from biome_manifest import ITEMS

COMFY = 'http://127.0.0.1:8188'
COMFY_INPUT = r'C:\ComfyUI\input'
OUT_DIR = r'C:\Zorion2\ai_drafts\biomes\batch_p1B'
SIL_DIR = r'C:\Zorion2\ai_drafts\biomes\silhouettes'
LOG = os.path.join(OUT_DIR, 'log.txt')
CKPT = 'juggernaut-xl-v9.safetensors'
CN = 'controlnet-canny-sdxl-1.0.safetensors'
NEG1 = 'text, watermark, blurry, low quality, deformed, ugly, duplicate, extra fingers, moon, sun, planet, star, satellite, celestial body'
NEG2 = 'text, watermark, blurry, low quality, deformed, ugly, duplicate, extra fingers, moon, sun, planet, star, satellite, celestial body, cluttered background, 3D render, depth of field, isometric'

STAGE2_BASE = 'miniature landscape scene, ENTIRE scene covered with rich painterly texture, soft gradients, atmospheric haze, subtle detail, clean empty sky, no sun, no moon, no planets, no stars, no celestial bodies, game icon, centered, on black background, no text, no watermark, maximal detail, masterpiece'
STAGE2_NORMAL = STAGE2_BASE + ', no orange, no flames'
STAGE2_LAVA = STAGE2_BASE + ', glowing lava'


def http_json(url, data=None, method=None):
    req = urllib.request.Request(url, data=data, method=method)
    with urllib.request.urlopen(req, timeout=30) as r:
        return json.loads(r.read().decode('utf-8'))


def post_prompt(workflow):
    body = json.dumps({'prompt': workflow}).encode('utf-8')
    resp = http_json(COMFY + '/prompt', data=body)
    pid = resp.get('prompt_id')
    if not pid:
        raise RuntimeError('no prompt_id: %s' % resp)
    return pid


def wait_result(pid, timeout=900):
    t0 = time.time()
    while time.time() - t0 < timeout:
        try:
            h = http_json(COMFY + '/history/' + pid)
            entry = h.get(pid)
            if entry and entry.get('status'):
                st = entry['status']
                if st.get('status_str') == 'error':
                    raise RuntimeError('comfy error: %s' % json.dumps(st)[:500])
                if st.get('status_str') == 'success' or st.get('completed'):
                    for node in entry.get('outputs', {}).values():
                        imgs = node.get('images')
                        if imgs:
                            return imgs[0]
                    return None
        except RuntimeError:
            raise
        except Exception:
            pass
        time.sleep(3)
    raise RuntimeError('timeout waiting for prompt %s' % pid)


def download(img_meta, out_path):
    url = '%s/view?filename=%s&subfolder=%s&type=%s' % (COMFY, img_meta['filename'], img_meta.get('subfolder', ''), img_meta['type'])
    with urllib.request.urlopen(url, timeout=60) as r, open(out_path, 'wb') as f:
        f.write(r.read())
    return out_path


def to_input(src):
    name = os.path.basename(src)
    dst = os.path.join(COMFY_INPUT, name)
    shutil.copyfile(src, dst)
    return name


def workflow_controlnet(prompt, neg, sil_path, seed):
    sil_name = to_input(sil_path)
    return {
        '1': {'class_type': 'CheckpointLoaderSimple', 'inputs': {'ckpt_name': CKPT}},
        '2': {'class_type': 'CLIPTextEncode', 'inputs': {'text': prompt, 'clip': ['1', 1]}},
        '3': {'class_type': 'CLIPTextEncode', 'inputs': {'text': neg, 'clip': ['1', 1]}},
        '8': {'class_type': 'LoadImage', 'inputs': {'image': sil_name}},
        '9': {'class_type': 'Canny', 'inputs': {'image': ['8', 0], 'low_threshold': 0.2, 'high_threshold': 0.5}},
        '10': {'class_type': 'ControlNetLoader', 'inputs': {'control_net_name': CN}},
        '11': {'class_type': 'ControlNetApply', 'inputs': {'conditioning': ['2', 0], 'control_net': ['10', 0], 'image': ['9', 0], 'strength': 1.5}},
        '4': {'class_type': 'VAEEncode', 'inputs': {'pixels': ['8', 0], 'vae': ['1', 2]}},
        '5': {'class_type': 'KSampler', 'inputs': {'seed': seed, 'steps': 40, 'cfg': 7.0, 'sampler_name': 'dpmpp_2m', 'scheduler': 'karras', 'denoise': 0.85, 'model': ['1', 0], 'positive': ['11', 0], 'negative': ['3', 0], 'latent_image': ['4', 0]}},
        '6': {'class_type': 'VAEDecode', 'inputs': {'samples': ['5', 0], 'vae': ['1', 2]}},
        '7': {'class_type': 'SaveImage', 'inputs': {'images': ['6', 0], 'filename_prefix': 'zorion_biome'}},
    }


def workflow_img2img(prompt, neg, img_path, seed, denoise=0.45):
    img_name = to_input(img_path)
    return {
        '1': {'class_type': 'CheckpointLoaderSimple', 'inputs': {'ckpt_name': CKPT}},
        '2': {'class_type': 'CLIPTextEncode', 'inputs': {'text': prompt, 'clip': ['1', 1]}},
        '3': {'class_type': 'CLIPTextEncode', 'inputs': {'text': neg, 'clip': ['1', 1]}},
        '8': {'class_type': 'LoadImage', 'inputs': {'image': img_name}},
        '4': {'class_type': 'VAEEncode', 'inputs': {'pixels': ['8', 0], 'vae': ['1', 2]}},
        '5': {'class_type': 'KSampler', 'inputs': {'seed': seed, 'steps': 40, 'cfg': 7.5, 'sampler_name': 'dpmpp_2m', 'scheduler': 'karras', 'denoise': denoise, 'model': ['1', 0], 'positive': ['2', 0], 'negative': ['3', 0], 'latent_image': ['4', 0]}},
        '6': {'class_type': 'VAEDecode', 'inputs': {'samples': ['5', 0], 'vae': ['1', 2]}},
        '7': {'class_type': 'SaveImage', 'inputs': {'images': ['6', 0], 'filename_prefix': 'zorion_biome'}},
    }


def log(msg):
    with open(LOG, 'a', encoding='utf-8') as f:
        f.write(msg + '\n')
    print(msg)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument('--start', type=int, default=1)
    ap.add_argument('--count', type=int, default=27)
    ap.add_argument('--seed0', type=int, default=50000)
    args = ap.parse_args()
    os.makedirs(OUT_DIR, exist_ok=True)

    end = min(args.start + args.count - 1, len(ITEMS))
    for i in range(args.start - 1, end):
        item = ITEMS[i]
        num = i + 1
        seed = args.seed0 + i * 3
        s1_path = os.path.join(OUT_DIR, '%d_stage1.png' % num)
        fin_path = os.path.join(OUT_DIR, '%d.png' % num)
        sil_path = os.path.join(SIL_DIR, item['sil'])
        stage2 = STAGE2_LAVA if item.get('lava') else STAGE2_NORMAL
        try:
            log('=== %d/%d [%s] seed=%d ===' % (num, len(ITEMS), item['id'], seed))
            pid = post_prompt(workflow_controlnet(item['s1'], NEG1, sil_path, seed))
            img1 = wait_result(pid)
            download(img1, s1_path)
            pid2 = post_prompt(workflow_img2img(stage2, NEG2, s1_path, seed + 500))
            img2 = wait_result(pid2)
            download(img2, fin_path)
            log('  ok %s (%d bytes)' % (item['id'], os.path.getsize(fin_path)))
        except Exception as e:
            log('  FAIL %s: %s' % (item['id'], e))
    log('=== batch done (start=%d count=%d) ===' % (args.start, args.count))


if __name__ == '__main__':
    main()