import { readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

const currentDir = dirname(fileURLToPath(import.meta.url));
const readFeatureSource = (relativePath: string) =>
  readFileSync(resolve(currentDir, relativePath), "utf8");

const pageSource = readFeatureSource(
  "../presentation/pages/GroupsPage.vue",
);
const editorSource = readFeatureSource(
  "../presentation/widgets/GroupEditorDialog.vue",
);
const coreFieldsSource = readFeatureSource(
  "../presentation/widgets/GroupEditorCoreFields.vue",
);
const antigravityFieldsSource = readFeatureSource(
  "../presentation/widgets/GroupEditorAntigravityFields.vue",
);
const providerFieldsSource = readFeatureSource(
  "../presentation/widgets/GroupEditorProviderFields.vue",
);
const accountRoutingFieldsSource = readFeatureSource(
  "../presentation/widgets/GroupEditorAccountRoutingFields.vue",
);
const createWrapperSource = readFeatureSource(
  "../presentation/widgets/CreateGroupDialog.vue",
);
const editWrapperSource = readFeatureSource(
  "../presentation/widgets/EditGroupDialog.vue",
);
const createControllerSource = readFeatureSource(
  "../presentation/composables/useCreateGroupController.ts",
);
const editControllerSource = readFeatureSource(
  "../presentation/composables/useEditGroupController.ts",
);

describe("groups editor modularization", () => {
  it("keeps create and edit as static wrappers over the shared editor", () => {
    expect(createWrapperSource).toContain(
      'import GroupEditorDialog from "./GroupEditorDialog.vue"',
    );
    expect(createWrapperSource).toContain(
      '<GroupEditorDialog mode="create" :context="context" />',
    );
    expect(editWrapperSource).toContain(
      '<GroupEditorDialog mode="edit" :context="context" />',
    );
    expect(createWrapperSource).not.toContain("import(");
    expect(editWrapperSource).not.toContain("import(");
  });

  it("preserves form ids, selectors, and controller-owned submit payloads", () => {
    expect(editorSource).toContain("'edit-group-form' : 'create-group-form'");
    expect(coreFieldsSource).toContain(
      "'edit-group-form-name' : 'group-form-name'",
    );
    expect(editorSource).toContain('data-tour="group-form-submit"');
    expect(editorSource).toContain('@submit.prevent="submit"');

    expect(createControllerSource).toContain("submit: handleCreateGroup");
    expect(createControllerSource).toContain("...formData");
    expect(createControllerSource).toContain(
      "profit_min_margin: profitControlEnabled",
    );
    expect(createControllerSource).toContain("await create(requestData)");
    expect(editControllerSource).toContain("submit: handleUpdateGroup");
    expect(editControllerSource).toContain("...formData");
    expect(editControllerSource).toContain(
      "profit_min_margin: profitControlEnabled",
    );
    expect(editControllerSource).toContain(
      "await update(editingGroup.value.id, payload)",
    );
    expect(pageSource).toContain("useCreateGroupController");
    expect(pageSource).toContain("useEditGroupController");
  });

  it("keeps field widgets static, ordered, and bound to the existing context", () => {
    const widgetNames = [
      "GroupEditorCoreFields",
      "GroupEditorAntigravityFields",
      "GroupEditorProviderFields",
      "GroupEditorAccountRoutingFields",
    ];
    let previousIndex = -1;
    for (const widgetName of widgetNames) {
      expect(editorSource).toContain(`import ${widgetName} from "./${widgetName}.vue"`);
      const currentIndex = editorSource.indexOf(`<${widgetName}`);
      expect(currentIndex).toBeGreaterThan(previousIndex);
      previousIndex = currentIndex;
    }

    const fieldSources = [
      coreFieldsSource,
      antigravityFieldsSource,
      providerFieldsSource,
      accountRoutingFieldsSource,
    ];
    for (const source of fieldSources) {
      expect(source).not.toContain("import(");
      expect(source).not.toContain("adminAPI");
      expect(source).toContain("GroupEditorDialogContext");
    }

    expect(coreFieldsSource).toContain('data-tour="group-form-platform"');
    expect(coreFieldsSource).toContain('data-tour="group-form-multiplier"');
    expect(coreFieldsSource).toContain('v-model="form.subscription_type"');
    expect(coreFieldsSource).toContain(':context="editorContext"');
    expect(antigravityFieldsSource).toContain("form.platform === 'antigravity'");
    expect(antigravityFieldsSource).toContain("@change=\"toggleScope('gemini_image')\"");
    expect(providerFieldsSource).toContain("form.platform === 'anthropic'");
    expect(providerFieldsSource).toContain("form.platform === 'openai'");
    expect(providerFieldsSource).toContain('@click="toggleLive()"');
    expect(accountRoutingFieldsSource).toContain(
      "form.require_oauth_only = !form.require_oauth_only",
    );
    expect(accountRoutingFieldsSource).toContain('v-model="form.fallback_group_id_on_invalid_request"');
    expect(accountRoutingFieldsSource).toContain('v-model="rule.pattern"');
    expect(accountRoutingFieldsSource).toContain('@input="searchAccountsByRule(rule)"');
  });

  it("uses one edit-platform watcher with the complete non-OpenAI reset", () => {
    expect(editControllerSource.match(/\(\) => editForm\.platform/g)).toHaveLength(1);

    const watcherStart = editControllerSource.indexOf("() => editForm.platform");
    const watcherEnd = editControllerSource.indexOf("\n  watch(", watcherStart + 1);
    const watcherSource = editControllerSource.slice(watcherStart, watcherEnd);

    expect(watcherSource).toContain("resetMessagesDispatchFormState(editForm)");
    expect(watcherSource).toContain('editForm.allow_live = false');
    expect(watcherSource).toContain('editForm.default_mapped_model = ""');
    expect(watcherSource).toContain(
      "editForm.fallback_group_id_on_invalid_request = null",
    );
  });
});
