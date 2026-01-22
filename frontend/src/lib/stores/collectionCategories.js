import { api } from '../api.js';
import { createCategoryStore } from './categoryStoreFactory.js';

// Categories store for collections
export const collectionCategoriesStore = createCategoryStore(api.collectionCategories, 'collection');
