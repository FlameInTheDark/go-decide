import { useCallback, useRef, useState } from "react"

interface DragState {
  from: number
  to: number
}

export function useDragReorder<T>(
  rows: T[],
  onChange: (rows: T[]) => void,
  disabled?: boolean,
) {
  const listRef = useRef<HTMLDivElement | null>(null)
  const [drag, setDrag] = useState<DragState | null>(null)

  const move = useCallback(
    (from: number, to: number) => {
      if (from === to) return
      const next = [...rows]
      const [item] = next.splice(from, 1)
      next.splice(to, 0, item)
      onChange(next)
      setDrag({ from, to })
    },
    [rows, onChange],
  )

  const start = useCallback(
    (event: React.PointerEvent<HTMLElement>, index: number) => {
      if (disabled) return
      event.preventDefault()
      const handle = event.currentTarget
      handle.setPointerCapture(event.pointerId)

      const centre = () => {
        const children = listRef.current?.children
        if (!children) return null
        return Array.from(children, (child) => {
          const rect = child.getBoundingClientRect()
          return { top: rect.top, middle: rect.top + rect.height / 2 }
        })
      }

      setDrag({ from: index, to: index })
      let current = index

      const onMove = (moveEvent: PointerEvent) => {
        const rects = centre()
        if (!rects) return
        const y = moveEvent.clientY
        let target = 0
        for (let i = 0; i < rects.length; i += 1) {
          if (y > rects[i].middle) target = i
        }
        if (target !== current) {
          current = target
          move(index, target)
          setDrag({ from: index, to: target })
        }
      }

      const onUp = () => {
        handle.releasePointerCapture?.(event.pointerId)
        handle.removeEventListener("pointermove", onMove)
        handle.removeEventListener("pointerup", onUp)
        handle.removeEventListener("pointercancel", onUp)
        setDrag(null)
      }

      handle.addEventListener("pointermove", onMove)
      handle.addEventListener("pointerup", onUp)
      handle.addEventListener("pointercancel", onUp)
    },
    [disabled, move],
  )

  const shift = useCallback(
    (index: number, delta: number) => {
      const to = Math.min(Math.max(index + delta, 0), rows.length - 1)
      if (to === index) return
      const next = [...rows]
      const [item] = next.splice(index, 1)
      next.splice(to, 0, item)
      onChange(next)
    },
    [rows, onChange],
  )

  return { listRef, drag, start, move, shift }
}