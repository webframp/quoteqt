# Future Ideas

Ideas for future improvements, not committed to.

## Quotes Management at Scale

Currently all quotes display on a single page. With 100s of quotes this will need:

### Pagination
- Server-side pagination (20-50 quotes per page)
- "Load more" button or infinite scroll
- Page number display and navigation

### Filtering & Search
- Text search across quote content
- Filter by civilization
- Filter by channel (for admins)
- Filter by date range (recently added)
- Combined filters

### Sorting
- Newest/oldest first
- Alphabetical
- By civilization
- By channel

### Bulk Operations
- Checkbox selection for multiple quotes
- Bulk delete
- Bulk assign to civilization
- Bulk move to different channel

### UI Improvements
- Collapsible quote cards (show just first line, expand on click)
- Table view option (more compact than cards)
- Keyboard shortcuts for power users (j/k navigation, x to select, d to delete)

### Performance
- Lazy load quote metadata
- Virtual scrolling for very large lists
- Cache aggressively on client side

### Quick Actions
- Inline edit (click to edit without modal/page reload)
- Swipe actions on mobile (swipe left to delete)
