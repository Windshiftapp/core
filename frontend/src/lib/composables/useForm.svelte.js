/**
 * Built-in validators for form fields
 */
export const validators = {
  /**
   * Validates that a field is not empty
   * @param {string} message - Error message to display
   * @returns {Function} Validator function
   */
  required: (message = 'This field is required') => (value) => {
    if (value === null || value === undefined) return message;
    if (typeof value === 'string' && value.trim() === '') return message;
    if (Array.isArray(value) && value.length === 0) return message;
    return null;
  },

  /**
   * Validates minimum string length
   * @param {number} min - Minimum length
   * @param {string} message - Error message
   * @returns {Function} Validator function
   */
  minLength: (min, message) => (value) => {
    if (!value) return null; // Let required handle empty
    const msg = message || `Must be at least ${min} characters`;
    return String(value).length >= min ? null : msg;
  },

  /**
   * Validates maximum string length
   * @param {number} max - Maximum length
   * @param {string} message - Error message
   * @returns {Function} Validator function
   */
  maxLength: (max, message) => (value) => {
    if (!value) return null;
    const msg = message || `Must be at most ${max} characters`;
    return String(value).length <= max ? null : msg;
  },

  /**
   * Validates email format
   * @param {string} message - Error message
   * @returns {Function} Validator function
   */
  email: (message = 'Invalid email address') => (value) => {
    if (!value) return null;
    const emailRegex = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
    return emailRegex.test(String(value)) ? null : message;
  },

  /**
   * Validates against a regex pattern
   * @param {RegExp} pattern - Regex pattern to match
   * @param {string} message - Error message
   * @returns {Function} Validator function
   */
  pattern: (pattern, message = 'Invalid format') => (value) => {
    if (!value) return null;
    return pattern.test(String(value)) ? null : message;
  },

  /**
   * Composes multiple validators into one
   * @param  {...Function} validatorFns - Validators to compose
   * @returns {Function} Combined validator function
   */
  compose: (...validatorFns) => (value) => {
    for (const validator of validatorFns) {
      const error = validator(value);
      if (error) return error;
    }
    return null;
  }
};

/**
 * Deep equality check for comparing form values
 * @param {*} a - First value
 * @param {*} b - Second value
 * @returns {boolean} Whether values are deeply equal
 */
function deepEqual(a, b) {
  if (a === b) return true;
  if (a === null || b === null) return a === b;
  if (typeof a !== typeof b) return false;

  if (typeof a === 'object') {
    if (Array.isArray(a) !== Array.isArray(b)) return false;

    const keysA = Object.keys(a);
    const keysB = Object.keys(b);

    if (keysA.length !== keysB.length) return false;

    for (const key of keysA) {
      if (!keysB.includes(key)) return false;
      if (!deepEqual(a[key], b[key])) return false;
    }

    return true;
  }

  return false;
}

/**
 * Deep clone an object
 * @param {*} obj - Object to clone
 * @returns {*} Cloned object
 */
function deepClone(obj) {
  if (obj === null || typeof obj !== 'object') return obj;
  if (Array.isArray(obj)) return obj.map(deepClone);

  const cloned = {};
  for (const key in obj) {
    if (Object.prototype.hasOwnProperty.call(obj, key)) {
      cloned[key] = deepClone(obj[key]);
    }
  }
  return cloned;
}

/**
 * Creates a reactive form composable with validation, submission handling, and dirty state tracking
 *
 * @param {Object} options - Form configuration options
 * @param {Object} options.initialValues - Initial form values
 * @param {Object} [options.schema] - Validation schema { fieldName: validatorFn }
 * @param {Function} [options.onSubmit] - Async function called on valid submission
 * @param {Object} [options.transforms] - Field transformations { fieldName: (value) => transformedValue }
 * @returns {Object} Form state and methods
 *
 * @example
 * const form = useForm({
 *   initialValues: { name: '', email: '' },
 *   schema: {
 *     name: validators.required('Name is required'),
 *     email: validators.compose(
 *       validators.required('Email is required'),
 *       validators.email('Invalid email')
 *     )
 *   },
 *   onSubmit: async (data) => {
 *     await api.create(data);
 *   }
 * });
 */
export function useForm({ initialValues, schema = {}, onSubmit, transforms = {} }) {
  // Store initial values for reset and dirty checking
  let storedInitialValues = deepClone(initialValues);

  // Reactive state
  let values = $state(deepClone(initialValues));
  let errors = $state({});
  let touched = $state({});
  let isSubmitting = $state(false);
  let submitError = $state(null);
  let submitSuccess = $state(false);

  // Derived state: check if any field has changed from initial
  const isDirty = $derived.by(() => {
    return !deepEqual(values, storedInitialValues);
  });

  // Derived state: check if form passes all validations
  const isValid = $derived.by(() => {
    for (const field in schema) {
      const validator = schema[field];
      if (validator) {
        const error = validator(values[field]);
        if (error) return false;
      }
    }
    return true;
  });

  /**
   * Validate a single field
   * @param {string} field - Field name to validate
   * @returns {string|null} Error message or null if valid
   */
  function validateField(field) {
    const validator = schema[field];
    if (!validator) return null;

    const error = validator(values[field]);
    errors[field] = error;
    return error;
  }

  /**
   * Run all validators and populate errors object
   * @returns {boolean} True if all fields are valid
   */
  function validate() {
    let allValid = true;
    const newErrors = {};

    for (const field in schema) {
      const validator = schema[field];
      if (validator) {
        const error = validator(values[field]);
        if (error) {
          newErrors[field] = error;
          allValid = false;
        }
      }
    }

    errors = newErrors;
    return allValid;
  }

  /**
   * Set a single field value
   * @param {string} field - Field name
   * @param {*} value - New value
   */
  function setValue(field, value) {
    // Apply transform if defined
    const transform = transforms[field];
    const finalValue = transform ? transform(value) : value;

    values[field] = finalValue;

    // Validate if field has been touched
    if (touched[field]) {
      validateField(field);
    }
  }

  /**
   * Mark a field as touched and run validation
   * @param {string} field - Field name
   */
  function touchField(field) {
    touched[field] = true;
    validateField(field);
  }

  /**
   * Check if a field has an error (and has been touched)
   * @param {string} field - Field name
   * @returns {boolean} Whether field has an error
   */
  function hasError(field) {
    return touched[field] && !!errors[field];
  }

  /**
   * Reset form to initial values or new values
   * @param {Object} [newValues] - Optional new initial values
   */
  function reset(newValues) {
    if (newValues) {
      storedInitialValues = deepClone(newValues);
    }
    values = deepClone(storedInitialValues);
    errors = {};
    touched = {};
    submitError = null;
    submitSuccess = false;
  }

  /**
   * Submit the form if valid
   * @returns {Promise<boolean>} True if submission succeeded
   */
  async function submit() {
    // Mark all fields as touched
    for (const field in schema) {
      touched[field] = true;
    }

    // Validate all fields
    if (!validate()) {
      return false;
    }

    if (!onSubmit) {
      return true;
    }

    isSubmitting = true;
    submitError = null;
    submitSuccess = false;

    try {
      await onSubmit(deepClone(values));
      submitSuccess = true;
      return true;
    } catch (e) {
      console.error('Form submission error:', e);
      submitError = e.message || 'Submission failed';
      return false;
    } finally {
      isSubmitting = false;
    }
  }

  /**
   * Get form data (clone of current values)
   * @returns {Object} Current form values
   */
  function getFormData() {
    return deepClone(values);
  }

  return {
    // State getters (reactive)
    get values() { return values; },
    get errors() { return errors; },
    get touched() { return touched; },
    get isDirty() { return isDirty; },
    get isValid() { return isValid; },
    get isSubmitting() { return isSubmitting; },
    get submitError() { return submitError; },
    get submitSuccess() { return submitSuccess; },

    // Methods
    setValue,
    touchField,
    hasError,
    validate,
    reset,
    submit,
    getFormData
  };
}
