// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { defineComponent, reactive } from "vue";
import { describe, expect, it, vi } from "vitest";

import SettingsManagedAccountsSection from "../src/components/SettingsManagedAccountsSection.vue";

const sectionHeaderStub = defineComponent({
  props: ["title", "description"],
  template: "<header><h2>{{ title }}</h2><p>{{ description }}</p><slot name='extra' /></header>",
});

const buttonStub = defineComponent({
  props: ["loading"],
  emits: ["click"],
  template: "<button type='button' :disabled='loading' @click='$emit(\"click\", $event)'><slot /></button>",
});

const chipStub = defineComponent({ template: "<span><slot /></span>" });
const emptyStateStub = defineComponent({ props: ["text"], template: "<p>{{ text }}</p>" });
const textFieldStub = defineComponent({
  props: ["modelValue"],
  emits: ["update:modelValue"],
  template: "<input :value='modelValue' @input='$emit(\"update:modelValue\", $event.target.value)' />",
});
const selectStub = defineComponent({
  props: ["modelValue"],
  emits: ["update:modelValue"],
  template: "<select :value='modelValue' @change='$emit(\"update:modelValue\", $event.target.value)'><option value='SIMULATE'>SIMULATE</option><option value='REAL'>REAL</option></select>",
});
const switchStub = defineComponent({
  props: ["modelValue"],
  emits: ["update:modelValue"],
  template: "<input type='checkbox' :checked='modelValue' @change='$emit(\"update:modelValue\", $event.target.checked)' />",
});

const stubs = {
  SectionHeader: sectionHeaderStub,
  "v-btn": buttonStub,
  "v-chip": chipStub,
  "v-empty-state": emptyStateStub,
  "v-text-field": textFieldStub,
  "v-select": selectStub,
  "v-switch": switchStub,
};

function account(id: string, enabled: boolean) {
  return {
    id,
    brokerId: "futu",
    accountId: `account-${id}`,
    displayName: `账户 ${id}`,
    tradingEnvironment: enabled ? "SIMULATE" : "REAL",
    market: "HK",
    securityFirm: "Futu Securities",
    enabled,
  };
}

function accountForm() {
  return reactive({
    brokerId: "futu",
    accountId: "account-a",
    displayName: "账户 a",
    tradingEnvironment: "SIMULATE",
    market: "HK",
    securityFirm: "Futu Securities",
    enabled: true,
  });
}

describe("SettingsManagedAccountsSection", () => {
  it("renders managed accounts and routes edit and delete actions to their owner", async () => {
    const populateAccountForm = vi.fn();
    const removeAccount = vi.fn();
    const wrapper = mount(SettingsManagedAccountsSection, {
      props: {
        accounts: [account("a", true), account("b", false)],
        accountForm: accountForm(),
        deletingAccountId: "",
        editingAccountId: null,
        savingAccount: false,
        populateAccountForm,
        removeAccount,
        resetAccountForm: vi.fn(),
        submitAccount: vi.fn(),
      } as never,
      global: { stubs },
    });

    expect(wrapper.text()).toContain("账户 a");
    expect(wrapper.text()).toContain("已启用");
    expect(wrapper.text()).toContain("已禁用");
    const buttons = wrapper.findAll("button");
    await buttons[0]?.trigger("click");
    await buttons[3]?.trigger("click");
    expect(populateAccountForm).toHaveBeenCalledWith(expect.objectContaining({ id: "a" }));
    expect(removeAccount).toHaveBeenCalledWith("b");
  });

  it("supports the editable form and shows an empty-state when no account is stored", async () => {
    const form = accountForm();
    const resetAccountForm = vi.fn();
    const submitAccount = vi.fn();
    const editing = mount(SettingsManagedAccountsSection, {
      props: {
        accounts: [account("a", true)],
        accountForm: form,
        deletingAccountId: "a",
        editingAccountId: "a",
        savingAccount: false,
        populateAccountForm: vi.fn(),
        removeAccount: vi.fn(),
        resetAccountForm,
        submitAccount,
      } as never,
      global: { stubs },
    });

    const textInputs = editing.findAll("input");
    await textInputs[0]?.setValue("changed-broker");
    await textInputs[1]?.setValue("changed-account");
    await textInputs[2]?.setValue("changed-display");
    await textInputs[3]?.setValue("US");
    await textInputs[4]?.setValue("changed-firm");
    await editing.find("select").setValue("REAL");
    await editing.findAll("input[type='checkbox']")[0]?.setValue(false);
    const editingButtons = editing.findAll("button");
    await editingButtons.at(-2)?.trigger("click");
    await editingButtons.at(-1)?.trigger("click");
    expect(form.brokerId).toBe("changed-broker");
    expect(form.accountId).toBe("changed-account");
    expect(form.displayName).toBe("changed-display");
    expect(form.market).toBe("US");
    expect(form.securityFirm).toBe("changed-firm");
    expect(form.tradingEnvironment).toBe("REAL");
    expect(form.enabled).toBe(false);
    expect(resetAccountForm).toHaveBeenCalledTimes(1);
    expect(submitAccount).toHaveBeenCalledTimes(1);

    const empty = mount(SettingsManagedAccountsSection, {
      props: {
        accounts: [],
        accountForm: accountForm(),
        deletingAccountId: "",
        editingAccountId: null,
        savingAccount: false,
        populateAccountForm: vi.fn(),
        removeAccount: vi.fn(),
        resetAccountForm: vi.fn(),
        submitAccount: vi.fn(),
      } as never,
      global: { stubs },
    });
    expect(empty.text()).toContain("当前还没有保存任何券商账户");
  });
});
