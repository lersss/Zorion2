// Сторож зацикливания: один и тот же вызов инструмента в одной сессии
// блокируется, когда перестаёт давать новые результаты (см. 100b).

export const LoopGuard = async ({ client }) => {
  const SAME_OUTPUT_STREAK = 4
  const SAME_ARGS_LIMIT = 40
  const MAX_BLOCKS = 5

  const sessions = new Map()

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

  const refuse = async (sessionID, b, reason) => {
    b.blocks += 1
    if (b.blocks > MAX_BLOCKS) {
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
    throw new Error(reason)
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
        await refuse(input.sessionID, b, stopMessage(input, times))
      }
      if (times > SAME_ARGS_LIMIT) {
        await refuse(input.sessionID, b, stopMessage(input, times))
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
