import { forms } from '../../api/forms.js';

export function loadPublicFormBootstrap(slug) {
  return forms.getBootstrap(slug);
}

export function loadPublicFormDetail(slug, formId) {
  return forms.getFormDetail(slug, formId);
}
