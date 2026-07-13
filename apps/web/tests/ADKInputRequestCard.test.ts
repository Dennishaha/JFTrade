// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { describe, expect, it, vi } from "vitest";

import ADKInputRequestCard from "@/components/adk-page/ADKInputRequestCard.vue";
import type { ADKInputRequest } from "@/contracts";

function request(status = "PENDING"): ADKInputRequest {
  return {
    id: "input-1",
    runId: "run-1",
    agentId: "agent-1",
    functionCallId: "call-1",
    title: "Choose",
    status,
    questions: [
      {
        id: "q1",
        question: "Mode?",
        allowOther: true,
        options: [
          { id: "q1-o1", label: "Safe", recommended: true },
          { id: "q1-o2", label: "Fast", description: "Less validation" },
        ],
      },
      {
        id: "q2",
        question: "Format?",
        allowOther: false,
        options: [
          { id: "q2-o1", label: "Markdown" },
          { id: "q2-o2", label: "JSON" },
        ],
      },
    ],
    answers: [],
    createdAt: "2026-07-12T00:00:00Z",
    updatedAt: "2026-07-12T00:00:00Z",
  };
}

describe("ADKInputRequestCard", () => {
  it("collects ordered answers and submits once when every question is answered", async () => {
    const submit = vi.fn().mockResolvedValue(undefined);
    const wrapper = mount(ADKInputRequestCard, { props: { request: request(), submit } });
    await wrapper.findAll(".adk-input-option")[1]!.trigger("click");
    await wrapper.findAll(".adk-input-card__actions button")[1]!.trigger("click");
    const submitButton = wrapper.findAll(".adk-input-card__actions button").at(-1)!;
    expect(submitButton.attributes("disabled")).toBeDefined();
    await wrapper.findAll(".adk-input-option")[0]!.trigger("click");
    expect(submitButton.attributes("disabled")).toBeUndefined();
    await submitButton.trigger("click");

    expect(submit).toHaveBeenCalledTimes(1);
    expect(submit).toHaveBeenCalledWith(
      expect.objectContaining({ id: "input-1" }),
      [
        { questionId: "q1", optionId: "q1-o2" },
        { questionId: "q2", optionId: "q2-o1" },
      ],
    );
  });

  it("treats other text as exclusive and keeps answered requests read-only", async () => {
    const submit = vi.fn().mockResolvedValue(undefined);
    const wrapper = mount(ADKInputRequestCard, { props: { request: request(), submit } });
    const textarea = wrapper.get("textarea");
    await textarea.setValue("Balanced");
    await wrapper.findAll(".adk-input-card__actions button")[1]!.trigger("click");
    await wrapper.findAll(".adk-input-option")[0]!.trigger("click");
    await wrapper.findAll(".adk-input-card__actions button").at(-1)!.trigger("click");
    expect(submit.mock.calls[0]?.[1]).toEqual([
      { questionId: "q1", otherText: "Balanced" },
      { questionId: "q2", optionId: "q2-o1" },
    ]);

    await wrapper.setProps({
      request: {
        ...request("ANSWERED"),
        answers: [
          { questionId: "q1", optionId: "q1-o1" },
          { questionId: "q2", optionId: "q2-o2" },
        ],
      },
    });
    expect(wrapper.find("footer").exists()).toBe(false);
    expect(wrapper.get("fieldset").attributes("disabled")).toBeDefined();
  });

  it("keeps drafts editable while moving backward and forward before submission", async () => {
    const wrapper = mount(ADKInputRequestCard, { props: { request: request(), submit: vi.fn() } });

    await wrapper.findAll(".adk-input-option")[0]!.trigger("click");
    await wrapper.findAll(".adk-input-card__actions button")[1]!.trigger("click");
    await wrapper.findAll(".adk-input-option")[1]!.trigger("click");
    await wrapper.findAll(".adk-input-card__actions button")[0]!.trigger("click");

    expect(wrapper.get("legend").text()).toBe("Mode?");
    expect(wrapper.findAll(".adk-input-option")[0]!.classes()).toContain("is-selected");
    await wrapper.findAll(".adk-input-option")[1]!.trigger("click");
    expect(wrapper.findAll(".adk-input-option")[1]!.classes()).toContain("is-selected");
  });

  it("does not mention a question count for a single question", () => {
    const single = request();
    single.questions = [single.questions[0]!];
    const wrapper = mount(ADKInputRequestCard, { props: { request: single, submit: vi.fn() } });

    expect(wrapper.find(".adk-input-card__question-nav").exists()).toBe(false);
    expect(wrapper.text()).not.toContain("1 个问题");
    expect(wrapper.text()).not.toContain("1 / 1");
  });

  it("disables editing and submission while a response is being resolved", async () => {
    const submit = vi.fn();
    const pending = request();
    pending.answers = [
      { questionId: "q1", optionId: "q1-o1" },
      { questionId: "q2", optionId: "q2-o1" },
    ];
    const wrapper = mount(ADKInputRequestCard, {
      props: { request: pending, busy: true, submit },
    });

    expect(wrapper.get("fieldset").attributes("disabled")).toBeDefined();
    const actionButtons = wrapper.findAll(".adk-input-card__actions button");
    expect(actionButtons.every((button) => button.attributes("disabled") !== undefined)).toBe(true);
    await actionButtons.at(-1)!.trigger("click");
    expect(submit).not.toHaveBeenCalled();
  });
});
