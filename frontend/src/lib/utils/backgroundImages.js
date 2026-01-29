/**
 * Preset background images for workspace customization
 * Each preset has an id, name, category, url, and thumbnail
 *
 * Images are sourced from Unsplash (free to use)
 * Using smaller sizes for thumbnails and optimized sizes for backgrounds
 */

export const backgroundCategories = [
    { id: 'abstract', name: 'Abstract' },
    { id: 'nature', name: 'Nature' },
    { id: 'minimal', name: 'Minimal' },
    { id: 'dark', name: 'Dark' }
];

export const backgroundPresets = [
    // Abstract
    {
        id: 'abstract-1',
        name: 'Gradient Waves',
        category: 'abstract',
        url: 'https://images.unsplash.com/photo-1557682250-33bd709cbe85?w=1920&q=80',
        thumbnail: 'https://images.unsplash.com/photo-1557682250-33bd709cbe85?w=200&q=60'
    },
    {
        id: 'abstract-2',
        name: 'Purple Nebula',
        category: 'abstract',
        url: 'https://images.unsplash.com/photo-1534796636912-3b95b3ab5986?w=1920&q=80',
        thumbnail: 'https://images.unsplash.com/photo-1534796636912-3b95b3ab5986?w=200&q=60'
    },
    {
        id: 'abstract-3',
        name: 'Colorful Smoke',
        category: 'abstract',
        url: 'https://images.unsplash.com/photo-1550684376-efcbd6e3f031?w=1920&q=80',
        thumbnail: 'https://images.unsplash.com/photo-1550684376-efcbd6e3f031?w=200&q=60'
    },
    {
        id: 'abstract-4',
        name: 'Blue Fluid',
        category: 'abstract',
        url: 'https://images.unsplash.com/photo-1579546929518-9e396f3cc809?w=1920&q=80',
        thumbnail: 'https://images.unsplash.com/photo-1579546929518-9e396f3cc809?w=200&q=60'
    },

    // Nature
    {
        id: 'nature-1',
        name: 'Mountain Lake',
        category: 'nature',
        url: 'https://images.unsplash.com/photo-1506905925346-21bda4d32df4?w=1920&q=80',
        thumbnail: 'https://images.unsplash.com/photo-1506905925346-21bda4d32df4?w=200&q=60'
    },
    {
        id: 'nature-2',
        name: 'Forest Path',
        category: 'nature',
        url: 'https://images.unsplash.com/photo-1448375240586-882707db888b?w=1920&q=80',
        thumbnail: 'https://images.unsplash.com/photo-1448375240586-882707db888b?w=200&q=60'
    },
    {
        id: 'nature-3',
        name: 'Ocean Sunset',
        category: 'nature',
        url: 'https://images.unsplash.com/photo-1507525428034-b723cf961d3e?w=1920&q=80',
        thumbnail: 'https://images.unsplash.com/photo-1507525428034-b723cf961d3e?w=200&q=60'
    },
    {
        id: 'nature-4',
        name: 'Northern Lights',
        category: 'nature',
        url: 'https://images.unsplash.com/photo-1531366936337-7c912a4589a7?w=1920&q=80',
        thumbnail: 'https://images.unsplash.com/photo-1531366936337-7c912a4589a7?w=200&q=60'
    },

    // Minimal
    {
        id: 'minimal-1',
        name: 'White Waves',
        category: 'minimal',
        url: 'https://images.unsplash.com/photo-1558591710-4b4a1ae0f04d?w=1920&q=80',
        thumbnail: 'https://images.unsplash.com/photo-1558591710-4b4a1ae0f04d?w=200&q=60'
    },
    {
        id: 'minimal-2',
        name: 'Soft Gradient',
        category: 'minimal',
        url: 'https://images.unsplash.com/photo-1557683316-973673baf926?w=1920&q=80',
        thumbnail: 'https://images.unsplash.com/photo-1557683316-973673baf926?w=200&q=60'
    },
    {
        id: 'minimal-3',
        name: 'Clean Lines',
        category: 'minimal',
        url: 'https://images.unsplash.com/photo-1553356084-58ef4a67b2a7?w=1920&q=80',
        thumbnail: 'https://images.unsplash.com/photo-1553356084-58ef4a67b2a7?w=200&q=60'
    },
    {
        id: 'minimal-4',
        name: 'Geometric',
        category: 'minimal',
        url: 'https://images.unsplash.com/photo-1509114397022-ed747cca3f65?w=1920&q=80',
        thumbnail: 'https://images.unsplash.com/photo-1509114397022-ed747cca3f65?w=200&q=60'
    },

    // Dark
    {
        id: 'dark-1',
        name: 'Dark Mountains',
        category: 'dark',
        url: 'https://images.unsplash.com/photo-1519681393784-d120267933ba?w=1920&q=80',
        thumbnail: 'https://images.unsplash.com/photo-1519681393784-d120267933ba?w=200&q=60'
    },
    {
        id: 'dark-2',
        name: 'Night Sky',
        category: 'dark',
        url: 'https://images.unsplash.com/photo-1475274047050-1d0c0975c63e?w=1920&q=80',
        thumbnail: 'https://images.unsplash.com/photo-1475274047050-1d0c0975c63e?w=200&q=60'
    },
    {
        id: 'dark-3',
        name: 'Dark Ocean',
        category: 'dark',
        url: 'https://images.unsplash.com/photo-1505142468610-359e7d316be0?w=1920&q=80',
        thumbnail: 'https://images.unsplash.com/photo-1505142468610-359e7d316be0?w=200&q=60'
    },
    {
        id: 'dark-4',
        name: 'Space',
        category: 'dark',
        url: 'https://images.unsplash.com/photo-1462331940025-496dfbfc7564?w=1920&q=80',
        thumbnail: 'https://images.unsplash.com/photo-1462331940025-496dfbfc7564?w=200&q=60'
    }
];

/**
 * Get presets by category
 * @param {string} categoryId - The category ID to filter by
 * @returns {Array} Array of preset objects in the specified category
 */
export function getPresetsByCategory(categoryId) {
    return backgroundPresets.filter(preset => preset.category === categoryId);
}

/**
 * Get a preset by ID
 * @param {string} presetId - The preset ID to find
 * @returns {Object|undefined} The preset object or undefined if not found
 */
export function getPresetById(presetId) {
    return backgroundPresets.find(preset => preset.id === presetId);
}
