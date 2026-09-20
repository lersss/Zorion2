# -*- coding: utf-8 -*-
# Раннер силуэтов пачки 2 (34 биома): запускает части A-E. Стиль B, без небесных тел.
import make_biome_sil_B2_a as a
import make_biome_sil_B2_b as b
import make_biome_sil_B2_c as c
import make_biome_sil_B2_d as d
import make_biome_sil_B2_e as e

if __name__ == '__main__':
    a.run()
    b.run()
    c.run()
    d.run()
    e.run()
    print('ALL STYLE-B2 SILHOUETTES DONE')
