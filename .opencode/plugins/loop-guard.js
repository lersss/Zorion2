// Сторож зацикливания: режет вызовы, которые перестали давать новое,
// и напоминает агенту про бюджет, когда он ушёл в спираль (идея 100b/100c/100d).
//
// Правила:
//  1) один и тот же вызов с неизменным результатом 5-й раз -> отказ;
//  2) тот же вызов с любым результатом 41-й раз       -> отказ;
//  3) серия отказов в сессии                          -> сессия гасится;
//  4) память >= 700k, или файл возвращается к уже виденному состоянию (5 раз),
//     или файл переписан 60 раз, или 15 прогонов семейства команд (тело команды
//     без служебного префикса кодировки — см. SERVICE_PREFIX),
//     или 3 одинаковых вывода агента подряд            -> напоминание в системную
//     память агента (один раз за сессию);
//  5) память >= 900k или 15 действий после напоминания -> отказ и гашение.
//
// Файл оценивается по РЕЗУЛЬТАТУ, а не по объёму (идея 100d): пока содержимое
// меняется — идёт работа; спираль — когда файл возвращается к уже виденному
// состоянию. Грубая страховка по объёму (60 правок одного файла) остаётся.
//
// Продление на лету (идея 100d): сервис учёта (tools/agent-dash, порт 8790) пишет
// .opencode/loop-guard.override.json — какой сессии и на какой срок продлить; сторож
// читает его на каждом действии, сбрасывает счётчик «после напоминания» и поднимает
// объёмный предел на extra. Продление только на срок, «навсегда» не бывает.
//
// Срабатывания показываются всплывашкой и пишутся в журнал (.opencode/loop-guard.log).

import fs from "node:fs"
import path from "node:path"

export const LoopGuard = async ({ client, directory }) => {
  const SAME_OUTPUT_STREAK = 4
  const SAME_ARGS_LIMIT = 40
  const MEM_WARN = 700000
  const MEM_STOP = 900000
  const SAME_FILE_EDITS = 60
  const SAME_FILE_REPEATS = 5
  const FILE_HASH_MIN_EDITS = 8
  const FILE_HASH_MAX_BYTES = 1000000
  const SAME_CMD_FAMILY = 15
  const SAME_CONCLUSION = 3
  const AFTER_REMINDER = 15
  const STOP_BLOCKS = 5
  const STOP_MEMORY_BLOCKS = 2
  const OVERRIDE_FILE = "loop-guard.override.json"

  const journalPath = directory ? path.join(directory, ".opencode", "loop-guard.log") : null
  const overridePath = directory ? path.join(directory, ".opencode", OVERRIDE_FILE) : null
  const sessions = new Map()
  let journalReady = false
  const overrides = { at: 0, list: {} }

  const journal = (sessionID, action, reason, extra = {}) => {
    if (!journalPath) return
    try {
      if (!journalReady) {
        fs.mkdirSync(path.dirname(journalPath), { recursive: true })
        journalReady = true
      }
      fs.appendFileSync(
        journalPath,
        JSON.stringify({ at: Date.now(), session: sessionID, action, reason, ...extra }) + "\n"
      )
    } catch {
      // журнал — необязательная запись, работе сторожа не мешает
    }
  }

  const bucket = (sessionID) => {
    let b = sessions.get(sessionID)
    if (!b) {
      b = {
        calls: new Map(),
        outputs: new Map(),
        files: new Map(),
        cmds: new Map(),
        blocks: new Map(),
        mem: 0,
        sameConclusion: 0,
        lastHead: "",
        currentMessage: "",
        currentHead: "",
        reminder: null,
        reminded: false,
        afterReminder: 0,
        extendedUntil: 0,
      }
      sessions.set(sessionID, b)
    }
    return b
  }

  const stable = (value) => {
    if (value === null || typeof value !== "object") return JSON.stringify(value) ?? "null"
    if (Array.isArray(value)) return "[" + value.map(stable).join(",") + "]"
    return (
      "{" +
      Object.keys(value)
        .sort()
        .map((k) => JSON.stringify(k) + ":" + stable(value[k]))
        .join(",") +
      "}"
    )
  }

  const hash = (text) => {
    let h = 5381
    for (let i = 0; i < text.length; i++) h = ((h << 5) + h + text.charCodeAt(i)) | 0
    return h
  }

  const head = (text) => String(text ?? "").replace(/\s+/g, " ").trim().slice(0, 40)
  const kilos = (v) => Math.round(v / 1000) + "k"

  // Служебная настройка кодировки в начале команды — не часть «тела». Семейство
  // считается по первым 48 символам, и общий префикс (\$OutputEncoding /
  // [Console]::OutputEncoding / chcp 65001) склеивал РАЗНЫЕ по смыслу команды в
  // одну серию (инцидент 2026-09-21: так легли сессии разработчика и менеджера).
  // ВАЖНО: имя типа содержит точки (`[Text.Encoding]::UTF8`,
  // `[System.Text.Encoding]::UTF8`) — `\w+` их не покрывал, префикс срезался
  // неполно (оставался `=`), и разные команды снова склеивались в одну семью
  // (инцидент 2026-09-22: на этом легла разведка менеджера). Тип — `[\w.]+`.
  const SERVICE_PREFIX =
    /^\s*(?:(?:\$OutputEncoding|\[Console\]::(?:Output|Input)Encoding)\s*=\s*\[[\w.]+\]::\w+\s*;?\s*|chcp\s+65001\s*;?\s*)+/i

  const commandFamily = (command) => {
    const full = String(command ?? "").trim()
    const body = full.replace(SERVICE_PREFIX, "").trim()
    return (body || full).slice(0, 48)
  }

  // Рабочая память сообщения: сколько модель держит в контексте на этом шаге.
  const memory = (tokens) => {
    if (!tokens) return 0
    const input = Number(tokens.input) || 0
    const cache = Number(tokens.cache?.read ?? tokens.cacheRead) || 0
    return input + cache
  }

  const fileState = (files, file) => {
    let s = files.get(file)
    if (!s) {
      s = { edits: 0, repeats: 0, seen: [] }
      files.set(file, s)
    }
    return s
  }

  // Отпечаток содержимого: повтор уже виденного состояния = крутимся на месте.
  const touchState = (s, text) => {
    if (text === null || text === undefined) return
    const seen = hash(String(text))
    if (s.seen.includes(seen)) s.repeats += 1
    else {
      s.seen.push(seen)
      if (s.seen.length > 40) s.seen.shift()
    }
  }

  // Содержимое файла для сверки; большие файлы не читаем.
  const readText = (file) => {
    try {
      if (fs.statSync(file).size > FILE_HASH_MAX_BYTES) return null
      return fs.readFileSync(file, "utf8")
    } catch {
      return null
    }
  }

  // Продление лимитов на лету (идея 100d): файл пишет сервис учёта, сторож читает.
  // Перечитываем только при изменении файла; продление действует, пока не истёк срок.
  const readOverrides = () => {
    if (!overridePath) return {}
    try {
      const at = fs.statSync(overridePath).mtimeMs
      if (at === overrides.at) return overrides.list
      overrides.at = at
      overrides.list = JSON.parse(fs.readFileSync(overridePath, "utf8")) || {}
    } catch {
      overrides.list = {}
    }
    return overrides.list
  }

  const extension = (sessionID) => {
    const e = readOverrides()[sessionID]
    const until = Number(e?.until)
    if (!e || !Number.isFinite(until) || until <= Date.now()) return null
    return { extra: Number(e.extra) || 0, until, by: String(e.by ?? ""), pardon: e.pardon === true }
  }

  const bump = (map, key) => {
    const n = (map.get(key) ?? 0) + 1
    map.set(key, n)
    return n
  }
  const over = (map, limit) => {
    let key = null
    let max = 0
    for (const [k, v] of map) if (v > max) [key, max] = [k, v]
    return max >= limit ? { key, max } : null
  }

  const toast = async (variant, title, message) => {
    try {
      await client.tui.showToast({
        body: { title, message, variant, duration: variant === "error" ? 15000 : 8000 },
        query: directory ? { directory } : undefined,
      })
    } catch {
      // приложение может не уметь показывать уведомления — работе это не мешает
    }
  }

  // Признак спирали: агент делает много действий без результата.
  const spiral = (b, extra = 0) => {
    if (b.mem >= MEM_WARN) return `рабочая память ${kilos(b.mem)}`;
    let looping = null;
    let biggest = null;
    for (const [file, s] of b.files) {
      if (s.repeats >= SAME_FILE_REPEATS && (!looping || s.repeats > looping.s.repeats))
        looping = { file, s };
      if (s.edits >= SAME_FILE_EDITS + extra && (!biggest || s.edits > biggest.s.edits))
        biggest = { file, s };
    }
    if (looping) return `файл ${looping.file} возвращался к уже виденному состоянию ${looping.s.repeats} раз`;
    if (biggest) return `файл ${biggest.file} переписан ${biggest.s.edits} раз (работа не сходится)`;
    const cmd = over(b.cmds, SAME_CMD_FAMILY)
    if (cmd) return `команда «${cmd.key}…» прогнана ${cmd.max} раз`;
    if (b.sameConclusion >= SAME_CONCLUSION) return `свой вывод повторён ${b.sameConclusion} раза подряд`;
    return null;
  }

  const report = (input, times) =>
    `Сторож зацикливания: бюджет исчерпан — вызов «${input.tool}» повторяется (${times} раз) без нового результата. ` +
    "Остановись и напиши отчёт: что сделано, что не вышло, какие остаются варианты. " +
    "Незаконченная задача с отчётом — нормальный результат, дальше перебирать действия — нет."

  const refuse = async (sessionID, b, kind, tool, reason, extra = {}) => {
    const count = (b.blocks.get(kind) ?? 0) + 1
    b.blocks.set(kind, count)
    const limit = kind === "memory" ? STOP_MEMORY_BLOCKS : STOP_BLOCKS
    const fatal = count > limit
    if (count === 1 || fatal) {
      await toast(
        fatal ? "error" : "warning",
        fatal ? "Сторож: сессия остановлена" : "Сторож: вызов обрезан",
        fatal
          ? `Агент повторял «${tool}» и не остановился — сессия погашена.`
          : `Агент повторяет «${tool}» — попросил сменить подход.`
      )
    }
    journal(sessionID, fatal ? "aborted" : "blocked", reason, { tool, ...extra })
    if (!fatal) throw new Error(reason)
    sessions.delete(sessionID)
    try {
      await client.session.abort({ path: { id: sessionID } })
    } catch {
      // сессия могла уже завершиться — это не ошибка
    }
    throw new Error(
      "Сторож зацикливания: сессия остановлена. Агент не прекратил повторять уже сделанное."
    )
  }

  return {
    event: async ({ event }) => {
      const type = event?.type
      const props = event?.properties ?? {}

      if (type === "session.deleted") {
        sessions.delete(props.info?.id ?? props.sessionID)
        return
      }

      if (type === "message.updated") {
        const info = props.info
        if (!info) return
        // Роль сообщения: в старом формате это role, в новом — type.
        if ((info.role ?? info.type) !== "assistant") return
        const mem = memory(info.tokens)
        if (mem <= 0) return
        bucket(info.sessionID ?? props.sessionID).mem = mem
        return
      }

      if (type === "message.part.updated") {
        const part = props.part
        if (part?.type !== "text" || !part.sessionID) return
        const b = bucket(part.sessionID)
        if (part.messageID !== b.currentMessage) {
          if (b.currentHead) {
            b.sameConclusion = b.currentHead === b.lastHead ? b.sameConclusion + 1 : 1
            b.lastHead = b.currentHead
          }
          b.currentMessage = part.messageID
        }
        b.currentHead = head(part.text)
      }
    },

    // Напоминание вкладывается прямо в системную память агента — до следующего шага.
    "experimental.chat.system.transform": async (input, output) => {
      const b = sessions.get(input?.sessionID)
      if (!b?.reminder || !Array.isArray(output?.system)) return
      output.system.push(b.reminder)
      b.reminder = null
    },

    "tool.execute.before": async (input, output) => {
      const b = bucket(input.sessionID)
      const sig = input.tool + "|" + stable(output.args)
      const times = bump(b.calls, sig)

      // Продление на лету: возвращаем бюджет действий и разрешаем напомнить снова.
      const ext = extension(input.sessionID)
      if (ext && ext.until !== b.extendedUntil) {
        b.extendedUntil = ext.until
        b.reminded = false
        b.afterReminder = 0
        journal(
          input.sessionID,
          "extend",
          `продление +${ext.extra} действий до ${new Date(ext.until).toLocaleTimeString()}`,
          { by: ext.by, extra: ext.extra, until: ext.until }
        )
      }

      // Индульгенция создателя (решение 2026-09-22): явное «этой вкладке работать»
      // отменяет сторожа для этой сессии — сбрасываем счётчики и пропускаем все
      // проверки ниже. Сторож по умолчанию работает как обычно; слушается только
      // явного указания. Пока индульгенция активна — сессию не гасим.
      if (ext && ext.pardon) {
        b.calls.clear()
        b.outputs.clear()
        b.cmds.clear()
        b.blocks.clear()
        b.sameConclusion = 0
        b.afterReminder = 0
        for (const s of b.files.values()) {
          s.edits = 0
          s.repeats = 0
          s.seen.length = 0
        }
        return
      }

      if (input.tool === "edit" || input.tool === "write" || input.tool === "apply_patch") {
        const file = output.args?.filePath ?? output.args?.path ?? "?"
        const s = fileState(b.files, file)
        s.edits += 1
        // write и apply_patch знают итоговое содержимое сразу; edit — сверяем после вызова
        const inline = input.tool === "write" ? output.args?.content : output.args?.patch
        if (typeof inline === "string") touchState(s, inline)
      } else if (input.tool === "bash" && typeof output.args?.command === "string") {
        bump(b.cmds, commandFamily(output.args.command))
      }

      // 1. Память на пределе — тормозим сразу.
      if (b.mem >= MEM_STOP) {
        await refuse(
          input.sessionID,
          b,
          "memory",
          input.tool,
          `Сторож зацикливания: рабочая память ${kilos(b.mem)} — предел для этой модели. ` +
            "Остановись и напиши отчёт: что сделано, что не вышло, какие остаются варианты.",
          { mem: b.mem }
        )
      }

      // 2. Тот же вызов с неизменным результатом.
      const same = b.outputs.get(sig)
      if (same && same.streak >= SAME_OUTPUT_STREAK) {
        await refuse(input.sessionID, b, "repeat", input.tool, report(input, times), { times })
      }
      if (times > SAME_ARGS_LIMIT) {
        await refuse(input.sessionID, b, "repeat", input.tool, report(input, times), { times })
      }

      // 3. Спираль: напоминаем один раз, дальше тормозим, если агент не свернул.
      const why = spiral(b, ext ? ext.extra : 0)
      if (!why) return
      if (!b.reminded) {
        b.reminded = true
        b.reminder =
          `[сторож Zorion] Бюджет исчерпан: ${why}. ` +
          "Заверши работу сейчас и напиши отчёт: что сделано, что не вышло, какие остаются варианты. " +
          "Незаконченная задача с отчётом — нормальный результат; продолжать перебор — нет."
        await toast("warning", "Сторож: бюджет исчерпан", `${why} — попросил агента сдать отчёт.`)
        journal(input.sessionID, "remind", why, { tool: input.tool, mem: b.mem })
        return
      }
      b.afterReminder += 1
      if (b.afterReminder > AFTER_REMINDER) {
        await refuse(
          input.sessionID,
          b,
          "spiral",
          input.tool,
          `Сторож зацикливания: после напоминания сделано ещё ${b.afterReminder} таких же действий (${why}). ` +
            "Остановись и напиши отчёт о сделанном и оставшихся вариантах.",
          { mem: b.mem }
        )
      }
    },

    "tool.execute.after": async (input, output) => {
      const b = bucket(input.sessionID)
      const sig = input.tool + "|" + stable(input.args)
      const current = hash(String(output.output ?? ""))
      const prev = b.outputs.get(sig)
      b.outputs.set(sig, {
        current,
        streak: prev && prev.current === current ? prev.streak + 1 : 1,
      })

      // Правку нельзя оценить по аргументам — смотрим, что стало с файлом.
      if (input.tool === "edit") {
        const file = input.args?.filePath ?? input.args?.path
        const s = file ? b.files.get(file) : null
        if (s && s.edits >= FILE_HASH_MIN_EDITS) touchState(s, readText(file))
      }
    },
  }
}
