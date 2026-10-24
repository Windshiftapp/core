import { milestoneCategories } from '../api/milestones.js';
import { createCategoryStore } from './categoryStoreFactory.js';

// Categories store for milestones
export const categoriesStore = createCategoryStore(milestoneCategories, 'milestone');
