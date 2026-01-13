<script>
  import { createEventDispatcher } from 'svelte';
  import { X, Download, ZoomIn, ZoomOut, RotateCw, Maximize2 } from 'lucide-svelte';

  const dispatch = createEventDispatcher();
  
  export let attachment = null;
  export let show = false;
  
  let imageElement = null;
  let scale = 1;
  let rotation = 0;
  let translateX = 0;
  let translateY = 0;
  let isDragging = false;
  let dragStartX = 0;
  let dragStartY = 0;
  let initialTranslateX = 0;
  let initialTranslateY = 0;
  
  // Reset transform when attachment changes
  $: if (attachment) {
    resetTransform();
  }
  
  function resetTransform() {
    scale = 1;
    rotation = 0;
    translateX = 0;
    translateY = 0;
  }
  
  function close() {
    show = false;
    resetTransform();
    dispatch('close');
  }
  
  async function download() {
    if (!attachment) return;
    
    try {
      const downloadUrl = `/api/attachments/${attachment.id}/download`;
      
      // Fetch the file as a blob
      const response = await fetch(downloadUrl);
      if (!response.ok) {
        throw new Error(`Download failed: ${response.statusText}`);
      }
      
      const blob = await response.blob();
      
      // Create a blob URL and download link
      const blobUrl = URL.createObjectURL(blob);
      const link = document.createElement('a');
      link.href = blobUrl;
      link.download = attachment.original_filename;
      link.style.display = 'none';
      
      document.body.appendChild(link);
      link.click();
      document.body.removeChild(link);
      
      // Clean up the blob URL
      URL.revokeObjectURL(blobUrl);
      
    } catch (error) {
      console.error('Download failed:', error);
      alert('Failed to download file: ' + error.message);
    }
  }
  
  function zoomIn() {
    scale = Math.min(scale * 1.25, 5);
  }
  
  function zoomOut() {
    scale = Math.max(scale * 0.8, 0.1);
  }
  
  function rotate() {
    rotation = (rotation + 90) % 360;
  }
  
  function fitToScreen() {
    if (!imageElement) return;
    
    const containerWidth = imageElement.parentElement.clientWidth - 40; // padding
    const containerHeight = imageElement.parentElement.clientHeight - 40;
    const imageWidth = imageElement.naturalWidth;
    const imageHeight = imageElement.naturalHeight;
    
    const scaleX = containerWidth / imageWidth;
    const scaleY = containerHeight / imageHeight;
    scale = Math.min(scaleX, scaleY, 1); // Don't scale up beyond 100%
    
    translateX = 0;
    translateY = 0;
  }
  
  function handleMouseDown(e) {
    if (e.button !== 0) return; // Only left click
    
    isDragging = true;
    dragStartX = e.clientX;
    dragStartY = e.clientY;
    initialTranslateX = translateX;
    initialTranslateY = translateY;
    
    e.preventDefault();
  }
  
  function handleMouseMove(e) {
    if (!isDragging) return;
    
    const deltaX = e.clientX - dragStartX;
    const deltaY = e.clientY - dragStartY;
    
    translateX = initialTranslateX + deltaX;
    translateY = initialTranslateY + deltaY;
  }
  
  function handleMouseUp() {
    isDragging = false;
  }
  
  function handleWheel(e) {
    e.preventDefault();
    
    const delta = e.deltaY > 0 ? 0.9 : 1.1;
    const newScale = Math.max(0.1, Math.min(5, scale * delta));
    
    // Zoom towards mouse position
    const rect = imageElement.getBoundingClientRect();
    const x = e.clientX - rect.left - rect.width / 2;
    const y = e.clientY - rect.top - rect.height / 2;
    
    translateX = translateX - x * (newScale - scale) / scale;
    translateY = translateY - y * (newScale - scale) / scale;
    
    scale = newScale;
  }
  
  function handleKeydown(e) {
    if (!show) return;
    
    switch (e.key) {
      case 'Escape':
        close();
        break;
      case '+':
      case '=':
        zoomIn();
        break;
      case '-':
        zoomOut();
        break;
      case 'r':
      case 'R':
        rotate();
        break;
      case '0':
        resetTransform();
        break;
      case 'f':
      case 'F':
        fitToScreen();
        break;
    }
  }
  
  // Get image source URL
  function getImageUrl(attachment) {
    if (!attachment) return '';
    return `/api/attachments/${attachment.id}/download`;
  }
  
  // Format file size
  function formatFileSize(bytes) {
    if (bytes === 0) return '0 Bytes';
    const k = 1024;
    const sizes = ['Bytes', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
  }
</script>

<svelte:window
  onkeydown={handleKeydown}
  onmousemove={handleMouseMove}
  onmouseup={handleMouseUp}
/>

{#if show && attachment}
  <!-- Modal backdrop -->
  <div
    class="fixed inset-0 bg-black bg-opacity-90 z-50 flex items-center justify-center"
    onclick={close}
  >
    <!-- Modal content -->
    <div
      class="relative w-full h-full flex flex-col"
      onclick={(e) => e.stopPropagation()}
    >
      <!-- Header -->
      <div class="flex items-center justify-between p-4 bg-black bg-opacity-50 text-white">
        <div class="flex-1 min-w-0">
          <h2 class="text-lg font-medium truncate">{attachment.original_filename}</h2>
          <p class="text-sm text-gray-300">
            {formatFileSize(attachment.file_size)} • {attachment.mime_type}
            {#if attachment.uploader_name}
              • Uploaded by {attachment.uploader_name}
            {/if}
          </p>
        </div>
        
        <!-- Controls -->
        <div class="flex items-center gap-2 ml-4">
          <button
            onclick={zoomOut}
            title="Zoom Out (-)"
            class="dark-button"
          >
            <ZoomOut class="w-4 h-4" />
          </button>
          
          <span class="text-sm text-gray-300 min-w-[4rem] text-center">
            {Math.round(scale * 100)}%
          </span>
          
          <button
            onclick={zoomIn}
            title="Zoom In (+)"
            class="dark-button"
          >
            <ZoomIn class="w-4 h-4" />
          </button>
          
          <button
            onclick={rotate}
            title="Rotate (R)"
            class="dark-button"
          >
            <RotateCw class="w-4 h-4" />
          </button>
          
          <button
            onclick={fitToScreen}
            title="Fit to Screen (F)"
            class="dark-button"
          >
            <Maximize2 class="w-4 h-4" />
          </button>
          
          <button
            onclick={download}
            title="Download"
            class="dark-button"
          >
            <Download class="w-4 h-4" />
          </button>
          
          <button
            onclick={close}
            title="Close (Esc)"
            class="dark-button"
          >
            <X class="w-4 h-4" />
          </button>
        </div>
      </div>
      
      <!-- Image container -->
      <div
        class="flex-1 overflow-hidden flex items-center justify-center p-4 cursor-move"
        class:cursor-grabbing={isDragging}
        onwheel={handleWheel}
      >
        <img
          bind:this={imageElement}
          src={getImageUrl(attachment)}
          alt={attachment.original_filename}
          class="max-w-none select-none transition-transform duration-200"
          style="transform: translate({translateX}px, {translateY}px) rotate({rotation}deg) scale({scale}); user-select: none;"
          onmousedown={handleMouseDown}
          onload={fitToScreen}
          draggable="false"
        />
      </div>
      
      <!-- Footer with shortcuts -->
      <div class="p-2 bg-black bg-opacity-50 text-center">
        <p class="text-xs text-gray-400">
          <span class="inline-block mx-2">Scroll: Zoom</span>
          <span class="inline-block mx-2">Drag: Pan</span>
          <span class="inline-block mx-2">R: Rotate</span>
          <span class="inline-block mx-2">F: Fit</span>
          <span class="inline-block mx-2">0: Reset</span>
          <span class="inline-block mx-2">Esc: Close</span>
        </p>
      </div>
    </div>
  </div>
{/if}

<style>
  .cursor-grabbing {
    cursor: grabbing !important;
  }
  
  .dark-button {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    padding: 8px;
    background-color: rgba(0, 0, 0, 0.7);
    color: white;
    border: 1px solid rgba(75, 85, 99, 0.8);
    border-radius: 6px;
    transition: all 0.2s ease;
    backdrop-filter: blur(4px);
  }
  
  .dark-button:hover {
    background-color: rgba(55, 65, 81, 0.9);
    border-color: rgba(156, 163, 175, 0.8);
    transform: translateY(-1px);
  }
  
  .dark-button:active {
    transform: translateY(0);
    background-color: rgba(31, 41, 55, 0.9);
  }
  
  .dark-button:focus {
    outline: none;
    box-shadow: 0 0 0 2px rgba(59, 130, 246, 0.5);
  }
</style>