import { describe, expect, it } from 'vitest';
import {
  canCreateRequirement,
  canCreateRequirementInAnyWorkspace,
  canEditRequirement,
  formatRequirementOwnerLabel,
  shouldConfirmTemplateReplace,
} from './requirementFormHelpers.js';

function mockPermissions({ isSystemAdmin = false, grants = {} } = {}) {
  return {
    isSystemAdmin,
    hasPermission: (workspaceId, permission) => grants[`${workspaceId}:${permission}`] === true,
  };
}

describe('canCreateRequirement', () => {
  it('allows system admin and page.create', () => {
    expect(canCreateRequirement(mockPermissions({ isSystemAdmin: true }), 1)).toBe(true);
    expect(canCreateRequirement(mockPermissions({ grants: { '1:page.create': true } }), 1)).toBe(
      true
    );
    expect(canCreateRequirement(mockPermissions(), 1)).toBe(false);
  });
});

describe('canEditRequirement', () => {
  it('allows page.edit and page.admin', () => {
    expect(canEditRequirement(mockPermissions({ grants: { '2:page.edit': true } }), 2)).toBe(true);
    expect(canEditRequirement(mockPermissions({ grants: { '2:page.admin': true } }), 2)).toBe(true);
    expect(canEditRequirement(mockPermissions(), 2)).toBe(false);
  });
});

describe('canCreateRequirementInAnyWorkspace', () => {
  it('returns true when any workspace allows create', () => {
    const perms = mockPermissions({ grants: { '3:page.create': true } });
    expect(canCreateRequirementInAnyWorkspace(perms, [{ id: 1 }, { id: 3 }])).toBe(true);
    expect(canCreateRequirementInAnyWorkspace(perms, [{ id: 1 }])).toBe(false);
  });
});

describe('formatRequirementOwnerLabel', () => {
  it('prefers full name then username then email', () => {
    expect(
      formatRequirementOwnerLabel({ first_name: 'Ada', last_name: 'Lovelace', username: 'ada' })
    ).toBe('Ada Lovelace');
    expect(formatRequirementOwnerLabel({ username: 'ada', email: 'ada@test' })).toBe('ada');
    expect(formatRequirementOwnerLabel(null)).toBe('');
  });
});

describe('shouldConfirmTemplateReplace', () => {
  it('requires confirmation when content was touched and non-empty', () => {
    expect(shouldConfirmTemplateReplace('hello', true)).toBe(true);
    expect(shouldConfirmTemplateReplace('hello', false)).toBe(false);
    expect(shouldConfirmTemplateReplace('   ', true)).toBe(false);
  });
});
