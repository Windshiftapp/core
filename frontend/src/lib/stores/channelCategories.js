import { api } from '../api.js';
import { createCategoryStore } from './categoryStoreFactory.js';

// Channel Categories store
export const channelCategoriesStore = createCategoryStore(api.channelCategories, 'channel');
