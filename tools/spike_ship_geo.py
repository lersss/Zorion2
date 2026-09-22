# -*- coding: utf-8 -*-
# СПАЙК 4 (продолжение 2026-09-21-ships-quality-rework §4–§5): объёмный рендер
# корабля под 3/4-ракурс. Тонкая обёртка над пакетом tools/spike_ship_geo/:
# корабль = набор 3D-деталей (призмы/боксы), ПЕРСПЕКТИВНАЯ камера с elevation
# (0=сверху) и лёгким yaw, ламберт по нормалям, фаски, z-buffer, AO, падающие
# тени, слой гриблзов и панельные линии. Кадрирование по геометрии (~80% кадра,
# фон (5,5,5)). Выход: цветной рендер 1536, карта глубины камеры 1536 (для
# ControlNet Depth), карта нормалей 1536.
# Детерминизм: random.Random(FNV1a(slug)). Только Python (numpy/Pillow/scipy).
# Использование: python spike_ship_geo.py --spec <in.json> --out <dir>
#                [--elev "0,15,25,40"] [--yaw 15]
import argparse
import json
import os

from spike_ship_geo import build_scene, render


def main():
    ap = argparse.ArgumentParser(description='Спайк 4: объёмный рендер кораблей (3/4-ракурс)')
    ap.add_argument('--spec', required=True)
    ap.add_argument('--out', required=True)
    ap.add_argument('--elev', default='0,15,25,40', help='углы возвышения камеры (0=сверху)')
    ap.add_argument('--yaw', type=float, default=15.0, help='поворот вокруг вертикали, °')
    ap.add_argument('--light', default='cine', choices=('cine', 'flat'),
                    help='освещение: cine (ключ+заполняющий+rim) или flat (один источник)')
    args = ap.parse_args()
    with open(args.spec, encoding='utf-8') as f:
        specs = json.load(f)
    os.makedirs(args.out, exist_ok=True)
    elevs = [float(x) for x in str(args.elev).split(',') if x.strip() != '']
    for spec in specs:
        sc = build_scene(spec)
        print('Greebles %s: %d' % (spec['race'], sc.greeble_count))
        for el in elevs:
            img, depth, nrm = render(spec, el, args.yaw, light=args.light)
            stem = '%s_e%02d' % (spec['race'], int(el))
            for kind, im in (('', img), ('_depth', depth), ('_normal', nrm)):
                path = os.path.join(args.out, '%s%s.png' % (stem, kind))
                im.save(path)
                print('Saved: %s' % path)


if __name__ == '__main__':
    main()
