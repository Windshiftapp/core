import { api } from '../api.js';
import { createCategoryStore } from './categoryStoreFactory.js';

// Categories store for milestones
export const categoriesStore = createCategoryStore(api.milestoneCategories, 'milestone');
