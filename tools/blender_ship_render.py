# -*- coding: utf-8 -*-
# СПАЙК 6: рендер-драйвер корабля из процедурного генератора
# a1studmuffin/SpaceshipGenerator (копия в C:\shipgen, вне репозитория Zorion).
# Запускается Blender'ом (не системным питоном):
#   blender.exe --background --factory-startup --python tools/blender_ship_render.py -- \
#       --seed 100001 --out ai_drafts/blender_ships/raw/humans [--size 1536] \
#       [--elev 25] [--yaw 15] [--gen C:\shipgen\SpaceshipGenerator] \
#       [--hull-min N --hull-max N --asym-min N --asym-max N \
#        --no-asym --no-hsym --vsym --no-bevel --no-detail]
# Выход в --out: color.png (a), depth.png (b, Z-pass, ближе = светлее),
# normal.png (c), meta.json.
# Пакетный режим (ОДИН запуск Blender на весь список — не плодить подпроцессы):
#       --tasks <json>  — массив {seed, out, label?, gen_args?}; gen_args — те же
#       флаги генератора (--hull-min и т.п.), свои для каждого корабля.
# Камера: 3/4 сверху (elev° над горизонтом, yaw° вокруг вертикали), нос вправо,
# ортографическая, автокадрирование ~80% кадра. Свет: ключ + заполняющий + контровой.
# Детерминизм: seed генератора фиксирован; одинаковый seed -> одинаковый результат.
import argparse
import json
import math
import os
import sys
import time

import bpy
from mathutils import Vector

MARK = "-- "


def add_gen_args(ap):
    """Флаги характера расы -> аргументы sg.generate_spaceship (см. generate_ship)."""
    ap.add_argument("--hull-min", type=int, default=None)
    ap.add_argument("--hull-max", type=int, default=None)
    ap.add_argument("--asym-min", type=int, default=None)
    ap.add_argument("--asym-max", type=int, default=None)
    ap.add_argument("--no-asym", action="store_true")
    ap.add_argument("--no-hsym", action="store_true")
    ap.add_argument("--vsym", action="store_true")
    ap.add_argument("--no-bevel", action="store_true")
    ap.add_argument("--no-detail", action="store_true")


def parse_args():
    argv = sys.argv
    idx = next((i for i, a in enumerate(argv) if a.endswith("blender_ship_render.py")), None)
    if idx is not None:
        argv = argv[idx + 1:]
    elif MARK in argv:
        argv = argv[argv.index(MARK) + 1:]
    else:
        argv = []
    if argv and argv[0] == "--":
        argv = argv[1:]
    ap = argparse.ArgumentParser(description="Рендер корабля из SpaceshipGenerator (Blender)")
    ap.add_argument("--seed", type=int, default=None)
    ap.add_argument("--out", default=None)
    ap.add_argument("--size", type=int, default=1536)
    ap.add_argument("--elev", type=float, default=25.0)
    ap.add_argument("--yaw", type=float, default=15.0)
    ap.add_argument("--gen", default=r"C:\shipgen\SpaceshipGenerator",
                    help="каталог копии SpaceshipGenerator")
    ap.add_argument("--tasks", default="", help="JSON-файл пакета задач (batch-режим)")
    add_gen_args(ap)
    args = ap.parse_args(argv)
    if not args.tasks and (args.seed is None or not args.out):
        ap.error("нужен --seed и --out, либо --tasks <json>")
    return args


def gen_kwargs(g):
    """Флаги характера расы -> kwargs generate_spaceship; имена сверены с сигнатурой."""
    kw = {}
    if g.hull_min is not None:
        kw["num_hull_segments_min"] = g.hull_min
    if g.hull_max is not None:
        kw["num_hull_segments_max"] = g.hull_max
    if g.asym_min is not None:
        kw["num_asymmetry_segments_min"] = g.asym_min
    if g.asym_max is not None:
        kw["num_asymmetry_segments_max"] = g.asym_max
    if g.no_asym:
        kw["create_asymmetry_segments"] = False
    if g.no_hsym:
        kw["allow_horizontal_symmetry"] = False
    if g.vsym:
        kw["allow_vertical_symmetry"] = True
    if g.no_bevel:
        kw["apply_bevel_modifier"] = False
    if g.no_detail:
        kw["create_face_detail"] = False
    return kw


def clean_scene():
    for obj in list(bpy.data.objects):
        bpy.data.objects.remove(obj, do_unlink=True)
    for coll in (bpy.data.meshes, bpy.data.materials, bpy.data.lights,
                 bpy.data.cameras, bpy.data.images):
        for block in list(coll):
            if block.users == 0:
                coll.remove(block)


def generate_ship(seed, gen_path, gen_opts=None):
    if gen_path not in sys.path:
        sys.path.insert(0, gen_path)
    import spaceship_generator as sg
    return sg.generate_spaceship(seed, **(gen_opts or {}))


def ship_corners(obj):
    return [obj.matrix_world @ Vector(c) for c in obj.bound_box]


def setup_camera(corners, size, elev_deg, yaw_deg, margin=0.8):
    center = sum(corners, Vector()) / 8.0
    max_dim = max(max(c[i] for c in corners) - min(c[i] for c in corners) for i in range(3))
    elev = math.radians(elev_deg)
    yaw = math.radians(yaw_deg)
    # Направление от центра к камере: yaw=0 — камера перед кораблём (сторона -Y),
    # yaw>0 — сдвиг к носу (+X). Так нос (+X) проецируется вправо, 3/4-ракурс.
    direction = Vector((math.sin(yaw) * math.cos(elev),
                        -math.cos(yaw) * math.cos(elev),
                        math.sin(elev)))
    dist = 4.0 * max_dim
    loc = center + direction * dist

    cam_data = bpy.data.cameras.new("ShipCam")
    cam_data.type = "ORTHO"
    cam = bpy.data.objects.new("ShipCam", cam_data)
    bpy.context.scene.collection.objects.link(cam)
    cam.location = loc
    cam.rotation_euler = (center - loc).to_track_quat("-Z", "Y").to_euler()
    bpy.context.view_layer.update()

    inv = cam.matrix_world.inverted()
    ux = uy = 0.0
    near, far = 1e18, -1e18
    for c in corners:
        p = inv @ c
        ux = max(ux, abs(p.x))
        uy = max(uy, abs(p.y))
        near = min(near, -p.z)
        far = max(far, -p.z)
    cam_data.ortho_scale = 2.0 * max(ux, uy) / margin
    cam_data.clip_start = max(0.01, near - max_dim)
    cam_data.clip_end = far + max_dim
    bpy.context.scene.camera = cam
    return cam, center, near, far


def add_sun(name, direction, energy, angle=0.2):
    data = bpy.data.lights.new(name, "SUN")
    data.energy = energy
    data.angle = angle
    obj = bpy.data.objects.new(name, data)
    bpy.context.scene.collection.objects.link(obj)
    obj.location = Vector(direction) * 10.0
    # Свет светит вдоль своей -Z; направляем из точки к началу координат.
    obj.rotation_euler = (Vector((0, 0, 0)) - obj.location).to_track_quat("-Z", "Y").to_euler()
    return obj


def setup_lights():
    add_sun("Key", (-1.0, -1.0, 1.2), 4.0, 0.25)   # ключевой: верх-слева-спереди
    add_sun("Fill", (1.4, -0.6, 0.15), 1.3, 0.5)   # заполняющий: справа
    add_sun("Rim", (0.3, 1.6, 0.9), 3.0, 0.3)      # контровой: сзади-сверху


def setup_world():
    world = bpy.context.scene.world
    if world is None:
        world = bpy.data.worlds.new("World")
        bpy.context.scene.world = world
    world.use_nodes = True
    bg = world.node_tree.nodes.get("Background")
    if bg:
        bg.inputs["Color"].default_value = (0.02, 0.025, 0.035, 1.0)
        bg.inputs["Strength"].default_value = 1.0


def pick_engine():
    """EEVEE Next headless; при ошибке — вторая попытка через Cycles/Workbench."""
    for engine in ("BLENDER_EEVEE_NEXT", "CYCLES", "BLENDER_WORKBENCH"):
        try:
            bpy.context.scene.render.engine = engine
            return engine
        except TypeError:
            continue
    raise RuntimeError("no render engine available")


def setup_render(scene, size, engine):
    scene.render.resolution_x = size
    scene.render.resolution_y = size
    scene.render.resolution_percentage = 100
    scene.render.film_transparent = True
    scene.render.image_settings.file_format = "PNG"
    scene.render.image_settings.color_mode = "RGBA"
    scene.render.image_settings.color_depth = "8"
    try:
        scene.view_settings.view_transform = "Standard"
        scene.view_settings.look = "None"
    except TypeError:
        pass
    vl = scene.view_layers[0]
    vl.use_pass_z = True
    vl.use_pass_normal = True
    if engine == "BLENDER_EEVEE_NEXT":
        scene.eevee.taa_render_samples = 64
    elif engine == "CYCLES":
        scene.cycles.samples = 48
        scene.cycles.use_denoising = True


def render_plain(path):
    scene = bpy.context.scene
    scene.use_nodes = False
    scene.render.filepath = path
    bpy.ops.render.render(write_still=True)


def render_composited(path, source_socket, build_fn):
    """Рендер с композитором: source_socket ('Depth'/'Normal') -> build_fn -> Composite."""
    scene = bpy.context.scene
    scene.use_nodes = True
    nt = scene.node_tree
    nt.nodes.clear()
    rl = nt.nodes.new("CompositorNodeRLayers")
    comp = nt.nodes.new("CompositorNodeComposite")
    out = build_fn(nt, rl)
    nt.links.new(out, comp.inputs["Image"])
    scene.render.filepath = path
    bpy.ops.render.render(write_still=True)


def build_depth(nt, rl, near, far):
    mr = nt.nodes.new("CompositorNodeMapRange")
    mr.inputs["From Min"].default_value = near
    mr.inputs["From Max"].default_value = far
    mr.inputs["To Min"].default_value = 1.0   # ближе = светлее
    mr.inputs["To Max"].default_value = 0.0
    try:
        mr.use_clamp = True
    except AttributeError:
        pass
    nt.links.new(rl.outputs["Depth"], mr.inputs["Value"])
    return mr.outputs["Value"]


def build_normal(nt, rl):
    # Normal хранит XYZ в [-1..1]; для картинки — *0.5 + 0.5 через MixRGB.
    half = (0.5, 0.5, 0.5, 1.0)
    mul = nt.nodes.new("CompositorNodeMixRGB")
    mul.blend_type = "MULTIPLY"
    mul.inputs["Fac"].default_value = 1.0
    mul.inputs[2].default_value = half
    add = nt.nodes.new("CompositorNodeMixRGB")
    add.blend_type = "ADD"
    add.inputs["Fac"].default_value = 1.0
    add.inputs[2].default_value = half
    nt.links.new(rl.outputs["Normal"], mul.inputs[1])
    nt.links.new(mul.outputs["Image"], add.inputs[1])
    return add.outputs["Image"]


def render_one(seed, out, size, elev, yaw, gen_path, gen_opts):
    """Сгенерировать и отрендерить один корабль. Возвращает meta-словарь."""
    os.makedirs(out, exist_ok=True)
    t0 = time.time()
    timings = {}

    clean_scene()
    t = time.time()
    obj = generate_ship(seed, gen_path, gen_opts)
    timings["generate"] = round(time.time() - t, 2)
    if obj is None or len(obj.data.vertices) == 0:
        raise RuntimeError("generator returned empty mesh")

    corners = ship_corners(obj)
    cam, center, near, far = setup_camera(corners, size, elev, yaw)
    setup_lights()
    setup_world()
    engine = pick_engine()
    setup_render(bpy.context.scene, size, engine)

    t = time.time()
    render_plain(os.path.join(out, "color.png"))
    timings["color"] = round(time.time() - t, 2)
    t = time.time()
    render_composited(os.path.join(out, "depth.png"), "Depth",
                      lambda nt, rl: build_depth(nt, rl, near, far))
    timings["depth"] = round(time.time() - t, 2)
    t = time.time()
    render_composited(os.path.join(out, "normal.png"), "Normal",
                      lambda nt, rl: build_normal(nt, rl))
    timings["normal"] = round(time.time() - t, 2)

    dims = [round(v, 4) for v in obj.dimensions]
    meta = {
        "seed": seed, "size": size, "elev": elev, "yaw": yaw,
        "engine": engine, "film_transparent": True,
        "camera": "orthographic 3/4 from above, nose right",
        "lights": {"key": 4.0, "fill": 1.3, "rim": 3.0, "type": "SUN"},
        "generator": {"path": gen_path, "opts": gen_opts, "verts": len(obj.data.vertices),
                      "polys": len(obj.data.polygons), "dimensions": dims,
                      "materials": [m.name for m in obj.data.materials]},
        "near": round(near, 4), "far": round(far, 4),
        "outputs": {"color": "color.png", "depth": "depth.png", "normal": "normal.png"},
        "timings": timings, "total": round(time.time() - t0, 2),
    }
    with open(os.path.join(out, "meta.json"), "w", encoding="utf-8") as f:
        json.dump(meta, f, ensure_ascii=False, indent=1)
    print("RENDER_OK engine=%s seed=%d verts=%d dims=%s total=%.1f"
          % (engine, seed, len(obj.data.vertices), dims, meta["total"]))
    return meta


def run_batch(args):
    """Пакет: ВСЕ корабли в одном процессе Blender (не по подпроцессу на сид)."""
    with open(args.tasks, encoding="utf-8") as f:
        tasks = json.load(f)
    metas = []
    for t in tasks:
        gp = argparse.ArgumentParser()
        add_gen_args(gp)
        g = gp.parse_args(t.get("gen_args", []))
        opts = gen_kwargs(g)
        m = {"seed": t["seed"], "out": t["out"], "label": t.get("label", ""),
             "gen_args": t.get("gen_args", []), "gen_opts": opts}
        try:
            m["meta"] = render_one(t["seed"], t["out"], args.size, args.elev, args.yaw,
                                   args.gen, opts)
            m["ok"] = True
        except Exception as e:
            m["ok"] = False
            m["error"] = "%s: %s" % (type(e).__name__, e)
            print("RENDER_FAIL seed=%d out=%s err=%s" % (t["seed"], t["out"], m["error"]))
        metas.append(m)
    print("BATCH_DONE ok=%d/%d" % (sum(1 for m in metas if m["ok"]), len(metas)))
    return metas


def main():
    args = parse_args()
    if args.tasks:
        run_batch(args)
        return
    render_one(args.seed, args.out, args.size, args.elev, args.yaw,
               args.gen, gen_kwargs(args))


if __name__ == "__main__":
    main()
