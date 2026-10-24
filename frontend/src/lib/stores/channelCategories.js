import { channelCategories } from '../api/channels.js';
import { createCategoryStore } from './categoryStoreFactory.js';

// Channel Categories store
export const channelCategoriesStore = createCategoryStore(channelCategories, 'channel');
