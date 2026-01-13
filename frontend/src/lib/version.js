import versionInfo from '../version.json';

const envCode = import.meta?.env?.VITE_APP_VERSION_CODE;
const envName = import.meta?.env?.VITE_APP_VERSION_NAME;

const versionCode = envCode && envCode.trim() !== '' ? envCode : versionInfo.code;
const versionName = envName && envName.trim() !== '' ? envName : versionInfo.name;

export const version = {
  code: versionCode,
  name: versionName
};

export const versionLabel = version.name ? `${version.code} "${version.name}"` : version.code;
