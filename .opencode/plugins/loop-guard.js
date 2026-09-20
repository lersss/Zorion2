// Сторож зацикливания: один и тот же вызов инструмента в одной сессии
// блокируется, когда перестаёт давать новые результаты (см. идею 100b).
// Срабатывания показываются всплывашкой и пишутся в журнал (.opencode/loop-guard.log).

import fs from "node:fs"
import path from "node:path"

export const LoopGuard = async ({ client, directory }) => {
  const SAME_OUTPUT_STREAK = 4
  const SAME_ARGS_LIMIT = 40
  const MAX_BLOCKS = 5

  const journalPath = directory ? path.join(directory, ".opencode", "loop-guard.log") : null
  const sessions = new Map()
  let journalReady = false

  const journal = (sessionID, tool, times, action) => {
    if (!journalPath) return
    try {
      if (!journalReady) {
        fs.mkdirSync(path.dirname(journalPath), { recursive: true })
        journalReady = true
      }
      fs.appendFileSync(
        journalPath,
        JSON.stringify({ at: Date.now(), session: sessionID, tool, times, action }) + "\n"
      )
    } catch {
      // журнал — необязательная запись, работе сторожа не мешает
    }
  }

  const bucket = (sessionID) => {
    let b = sessions.get(sessionID)
    if (!b) {
      b = { calls: new Map(), outputs: new Map(), blocks: 0 }
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

  const refuse = async (sessionID, b, tool, times, reason) => {
    b.blocks += 1
    const fatal = b.blocks > MAX_BLOCKS
    if (b.blocks === 1 || fatal) {
      await toast(
        fatal ? "error" : "warning",
        fatal ? "Сторож: сессия остановлена" : "Сторож: вызов обрезан",
        fatal
          ? `Агент повторял «${tool}» ${times} раз без нового результата — сессия погашена.`
          : `Агент повторяет «${tool}» (${times} раз) без нового результата — попросил сменить подход.`
      )
    }
    journal(sessionID, tool, times, fatal ? "aborted" : "blocked")
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

  const stopMessage = (input, times) =>
    `Сторож зацикливания: вызов «${input.tool}» повторяется (${times} раз) и не даёт нового результата. ` +
    "Не повторяй его. Измени подход — или остановись и напиши отчёт: что сделано, что осталось, в чём именно застрял."

  return {
    event: async ({ event }) => {
      if (event?.type === "session.deleted") {
        sessions.delete(event.properties?.info?.id ?? event.properties?.sessionID)
      }
    },
    "tool.execute.before": async (input, output) => {
      const b = bucket(input.sessionID)
      const sig = input.tool + "|" + stable(output.args)
      const times = (b.calls.get(sig) ?? 0) + 1
      b.calls.set(sig, times)
      const same = b.outputs.get(sig)
      if (same && same.streak >= SAME_OUTPUT_STREAK) {
        await refuse(input.sessionID, b, input.tool, times, stopMessage(input, times))
      }
      if (times > SAME_ARGS_LIMIT) {
        await refuse(input.sessionID, b, input.tool, times, stopMessage(input, times))
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
