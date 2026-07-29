import { nextTick, type Ref } from "vue";

export async function scrollToBottom(threadRef: Ref<HTMLElement | null>): Promise<void> {
  await nextTick();
  if (threadRef.value) {
    threadRef.value.scrollTop = threadRef.value.scrollHeight;
  }
}
