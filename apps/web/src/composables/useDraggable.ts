import { onUnmounted, reactive } from "vue";

export interface DragState {
  x: number;
  y: number;
  dragging: boolean;
}

/**
 * 使一个元素可以通过 mousedown 事件拖拽。
 * 使用 transform: translate(x, y) 相对于元素的 CSS 初始位置进行偏移，
 * 不破坏原有 CSS 定位。
 *
 * 用法：
 *   const { drag, startDrag } = useDraggable()
 *   <div :style="{ transform: `translate(${drag.x}px, ${drag.y}px)` }"
 *        @mousedown="startDrag" />
 */
export function useDraggable() {
  const drag = reactive<DragState>({ x: 0, y: 0, dragging: false });

  let startMouseX = 0;
  let startMouseY = 0;
  let startPosX = 0;
  let startPosY = 0;

  function onMouseMove(e: MouseEvent) {
    drag.x = startPosX + (e.clientX - startMouseX);
    drag.y = startPosY + (e.clientY - startMouseY);
  }

  function onMouseUp() {
    drag.dragging = false;
    document.removeEventListener("mousemove", onMouseMove);
    document.removeEventListener("mouseup", onMouseUp);
  }

  function startDrag(e: MouseEvent) {
    if (e.button !== 0) return;
    drag.dragging = true;
    startMouseX = e.clientX;
    startMouseY = e.clientY;
    startPosX = drag.x;
    startPosY = drag.y;
    e.preventDefault();
    document.addEventListener("mousemove", onMouseMove);
    document.addEventListener("mouseup", onMouseUp);
  }

  onUnmounted(onMouseUp);

  return { drag, startDrag };
}
