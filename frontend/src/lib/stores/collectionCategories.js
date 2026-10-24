import { collectionCategories } from '../api/collections.js';
import { createCategoryStore } from './categoryStoreFactory.js';

// Categories store for collections
export const collectionCategoriesStore = createCategoryStore(collectionCategories, 'collection');
