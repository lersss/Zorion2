# QA-прогон: минимальный A/B-фикс вариаций в Go-арт-студии (cmd/art-studio)

**Дата:** 2026-09-17
**Тестировщик:** @tester
**Вектор:** Визуал (арт-студия, спека 67a.1)

## Прогнал

- `go build ./...` — OK
- `go vet ./...` — OK
- `go test ./cmd/art-studio/...` — OK (все пакеты зелёные: art-studio, config, generator, handlers, postproc; comfy — нет тестов)
- `curl.exe http://127.0.0.1:8188/object_info/{ImageToMask,ThresholdMask,MaskToImage}` — 200/200/200
- `curl.exe http://127.0.0.1:8798/status` — `{"running":false,"done":0,"total":0,"current":""}` (валидный JSON)
- `curl.exe http://127.0.0.1:8798/` — HTML содержит `value="0.7"` у ползунка denoise

## Сценарии (контрольная проверка A/B-фикса, только чтение)

| Пункт | Что проверено | Результат |
|---|---|---|
| 1. workflow.go Img2ImgWorkflow | LoadImage id8 → ImageToMask id9 (alpha) → ThresholdMask id10 (0.5) → MaskToImage id11 → Canny id12 (0.2/0.5) → ControlNetLoader id13 → ControlNetApplyAdvanced id14 (strength 1.2, start 0.0, end 0.7, positive ["2",0], negative ["3",0], control_net ["13",0], image ["12",0]); KSampler positive ["14",0], negative ["14",1], latent от VAEEncode id4 (эталон), denoise из параметра | ✅ (workflow.go:33-45) |
| 2. prompt.go BuildPrompt | form = race.Forms (узкий список расы, НЕ RandomForm); character/parts из fc.Character/fc.Parts; строка "abstract structure" (не "creature"); LightNeg() без запретов материала/цвета/creature | ✅ (prompt.go:81-88, 94-96) |
| 3. race_job.go / ref_job.go | genVarJob передаёт LightNeg() в Img2ImgWorkflow (вариации); genRefJob — fam.Neg в Txt2ImgWorkflow (не тронут) | ✅ (race_job.go:70, ref_job.go:51) |
| 4. index.html | ползунок denoise value="0.7" (min 0.3, max 0.75, step 0.05), лейбл 0.7, /genvar передаёт denoise | ✅ (index.html:49-50, 190-192) |
| 5. DoD | go build / go vet / go test ./cmd/art-studio/... — зелёные | ✅ |
| 6. ComfyUI | /object_info/ImageToMask, /object_info/ThresholdMask, /object_info/MaskToImage — 200 (классы есть) | ✅ |
| 7. Студия жива | /status → валидный JSON; / → HTML содержит denoise 0.7 | ✅ |

## Вердикт

ПРОЙДЕНО. Все 7 пунктов контрольной проверки ОК. Код соответствует арт-ТЗ 67a:
Canny от силуэта (маска → белое на чёрном, не рендер), форма из узкого списка
расы, деталь-ось character/parts, «abstract structure», лёгкий негатив без
запретов материала/цвета/creature, denoise 0.7. Готово к живой проверке создателя.

## Найденное

Багов не найдено.

## Предложения в чек-лист (для @manager)

- Новая зона «арт-студия» (cmd/art-studio) в QA_CHECKLIST.md: кейс «вариации расы
  от эталона: силуэт = эталон (Canny от маски), текстуры разные, denoise 0.7» —
  живой прогон /genvar (пачка 8), глазами создателя.

## Не проверено (граница покрытия)

- Живой прогон пачки 8 вариаций (запрещено заданием: ComfyUI живой, генерацию не запускать).
  Нужно создателю: открыть http://127.0.0.1:8798, выбрать семейство F2 и расу
  (эталоны ref_F2_r{5,6,7,8,9,48}.png на месте), /genvar?fam=F2&race=<id>&n=8&denoise=0.7,
  глазами оценить «силуэт = эталон, текстуры разные».
- Визуальный результат (держит ли силуэт, разнообразие текстур) — только глазами создателя.
- Эффективность ControlNetApplyAdvanced end_percent 0.7 vs denoise 0.7 — эмпирика живого прогона.