// Сторож зацикливания: режет вызовы, которые перестали давать новое,
// и напоминает агенту про бюджет, когда он ушёл в спираль (идея 100b/100c).
//
// Правила:
//  1) один и тот же вызов с неизменным результатом 5-й раз -> отказ;
//  2) тот же вызов с любым результатом 41-й раз       -> отказ;
//  3) серия отказов в сессии                          -> сессия гасится;
//  4) память >= 700k или 25 переписываний файла, 15 прогонов семейства команд,
//     3 одинаковых вывода агента подряд                -> напоминание в системную
//     память агента (один раз за сессию);
//  5) память >= 900k или 15 действий после напоминания -> отказ и гашение.
//
// Срабатывания показываются всплывашкой и пишутся в журнал (.opencode/loop-guard.log).

import fs from "node:fs"
import path from "node:path"

export const LoopGuard = async ({ client, directory }) => {
  const SAME_OUTPUT_STREAK = 4
  const SAME_ARGS_LIMIT = 40
  const MEM_WARN = 700000
  const MEM_STOP = 900000
  const SAME_FILE_EDITS = 25
  const SAME_CMD_FAMILY = 15
  const SAME_CONCLUSION = 3
  const AFTER_REMINDER = 15
  const STOP_BLOCKS = 5
  const STOP_MEMORY_BLOCKS = 2

  const journalPath = directory ? path.join(directory, ".opencode", "loop-guard.log") : null
  const sessions = new Map()
  let journalReady = false

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
        edits: new Map(),
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
  const spiral = (b) => {
    if (b.mem >= MEM_WARN) return `рабочая память ${kilos(b.mem)}`;
    const file = over(b.edits, SAME_FILE_EDITS)
    if (file) return `файл ${file.key} переписан ${file.max} раз`;
    const cmd = over(b.cmds, SAME_CMD_FAMILY)
    if (cmd) return `команда «${cmd.key}…» прогнана ${cmd.max} раз`;
    if (b.sameConclusion >= SAME_CONCLUSION) return `свой вывод повторён ${b.sameConclusion} раза подряд`;
    return null
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
        if (info?.role !== "assistant" || !info.tokens) return
        const b = bucket(info.sessionID ?? props.sessionID)
        b.mem = (info.tokens.input ?? 0) + (info.tokens.cache?.read ?? 0)
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

      if (input.tool === "edit" || input.tool === "write" || input.tool === "apply_patch") {
        const file = output.args?.filePath ?? output.args?.path ?? "?"
        bump(b.edits, file)
      } else if (input.tool === "bash" && typeof output.args?.command === "string") {
        bump(b.cmds, output.args.command.trim().slice(0, 48))
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
      const why = spiral(b)
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
    },
  }
}
