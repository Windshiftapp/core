import { portal } from '../api/portal.js';
import { confirm } from '../composables/useConfirm.js';
import { navigate } from '../router.js';
import { authStore } from './auth.svelte.js';
import { portalAuthStore } from './portalAuth.svelte.js';
import { errorToast } from './toasts.svelte.js';

let context = {
  closeProfileMenu: () => {},
  getSlug: () => null,
};

let showRequests = $state(false);
let requests = $state([]);
let loadingRequests = $state(false);
let selectedRequest = $state(null);
let comments = $state([]);
let loadingComments = $state(false);
let newComment = $state('');
let addingComment = $state(false);
let attachments = $state([]);
let uploadingAttachment = $state(false);

let showDrafts = $state(false);
let drafts = $state([]);
let loadingDrafts = $state(false);

let showApprovals = $state(false);
let approvals = $state([]);
let loadingApprovals = $state(false);
let selectedApproval = $state(null);
let selectedApprovalRequest = $state(null);
let loadingApprovalDetail = $state(false);
let approvalComment = $state('');
let decidingApproval = $state(false);

let requestsLoadId = 0;
let commentsLoadId = 0;
let attachmentsLoadId = 0;
let draftsLoadId = 0;
let approvalsLoadId = 0;
let approvalDetailLoadId = 0;

function isAuthenticated() {
  return authStore.isAuthenticated || portalAuthStore.isAuthenticated;
}

export function configurePortalActivityStore(nextContext) {
  context = nextContext;
}

function selectView(view) {
  showRequests = view === 'requests';
  showDrafts = view === 'drafts';
  showApprovals = view === 'approvals';
  if (view !== 'requests') selectedRequest = null;
  if (view !== 'approvals') {
    selectedApproval = null;
    selectedApprovalRequest = null;
  }
}

async function loadRequests() {
  const slug = context.getSlug();
  if (!isAuthenticated() || !slug) return;
  const loadId = ++requestsLoadId;
  loadingRequests = true;
  try {
    const result = await portal.getMyRequests(slug);
    if (loadId === requestsLoadId) requests = result || [];
  } catch (err) {
    console.error('Failed to load requests:', err);
  } finally {
    if (loadId === requestsLoadId) loadingRequests = false;
  }
}

async function loadComments(itemId) {
  const slug = context.getSlug();
  if (!slug) return;
  const loadId = ++commentsLoadId;
  loadingComments = true;
  try {
    const result = await portal.getRequestComments(slug, itemId);
    if (loadId === commentsLoadId) comments = result || [];
  } catch (err) {
    console.error('Failed to load comments:', err);
  } finally {
    if (loadId === commentsLoadId) loadingComments = false;
  }
}

async function loadAttachments(itemId) {
  const slug = context.getSlug();
  if (!slug) return;
  const loadId = ++attachmentsLoadId;
  try {
    const result = await portal.getRequestAttachments(slug, itemId);
    if (loadId === attachmentsLoadId) attachments = result || [];
  } catch (err) {
    // Attachments stay empty on failure; the timeline must still render.
    console.error('Failed to load attachments:', err);
    if (loadId === attachmentsLoadId) attachments = [];
  }
}

async function addAttachment(file) {
  const slug = context.getSlug();
  if (!file || !selectedRequest || !slug) return;
  const requestId = selectedRequest.id;
  try {
    uploadingAttachment = true;
    const result = await portal.addRequestAttachment(slug, requestId, file);
    // The upload endpoint answers with an AttachmentUploadResponse wrapper;
    // list entries are bare attachment objects.
    const attachment = result?.attachment ?? result;
    if (selectedRequest?.id === requestId) {
      attachments = [...attachments, attachment];
    }
  } catch (err) {
    console.error('Failed to upload attachment:', err);
    if (selectedRequest?.id === requestId) {
      errorToast(err?.message || 'Failed to upload file. Please try again.');
    }
  } finally {
    uploadingAttachment = false;
  }
}

async function viewRequest(request) {
  selectedRequest = request;
  navigate(`/portal/${context.getSlug()}?view=requests&id=${request.id}`);
  await Promise.all([loadComments(request.id), loadAttachments(request.id)]);
}

async function loadAndViewRequest(requestId) {
  const slug = context.getSlug();
  if (!slug) return;
  try {
    let request = await portal.getRequestDetail(slug, requestId);
    // A merged duplicate is a redirect to its canonical (WI-1528). Follow a
    // bounded chain so a stale deep link lands on the live thread. When the
    // canonical is not readable here (a cross-requester merge keeps the
    // content on the source), fall back to the duplicate itself.
    for (let hops = 0; request?.merged_into_item_id && hops < 5; hops += 1) {
      try {
        request = await portal.getRequestDetail(slug, request.merged_into_item_id);
      } catch {
        request = await portal.getRequestDetail(slug, requestId);
        break;
      }
    }
    selectedRequest = request;
    await Promise.all([loadComments(request.id), loadAttachments(request.id)]);
  } catch (err) {
    console.error('Failed to load request:', err);
  }
}

async function addComment() {
  const slug = context.getSlug();
  if (!newComment.trim() || !selectedRequest || !slug) return;
  const requestId = selectedRequest.id;
  const draft = newComment;
  try {
    addingComment = true;
    const comment = await portal.addRequestComment(slug, requestId, draft);
    // The user may have switched requests while the POST was in flight; apply
    // the result only to the request it belongs to.
    if (selectedRequest?.id !== requestId) return;
    comments = [...comments, comment];
    if (newComment === draft) newComment = '';
  } catch (err) {
    console.error('Failed to add comment:', err);
    errorToast('Failed to add comment. Please try again.');
  } finally {
    addingComment = false;
  }
}

function closeRequestDetail() {
  attachmentsLoadId += 1;
  selectedRequest = null;
  comments = [];
  newComment = '';
  attachments = [];
  navigate(`/portal/${context.getSlug()}?view=requests`);
}

function setShowRequests(value) {
  if (value) {
    selectView('requests');
    if (isAuthenticated()) void loadRequests();
  } else {
    showRequests = false;
    selectedRequest = null;
  }
}

async function toggleRequests() {
  context.closeProfileMenu();
  if (showRequests) {
    showRequests = false;
    selectedRequest = null;
    comments = [];
    navigate(`/portal/${context.getSlug()}`);
    return;
  }
  selectView('requests');
  navigate(`/portal/${context.getSlug()}?view=requests`);
  if (isAuthenticated()) await loadRequests();
}

async function loadDrafts() {
  const slug = context.getSlug();
  if (!isAuthenticated() || !slug) return;
  const loadId = ++draftsLoadId;
  loadingDrafts = true;
  try {
    const result = await portal.drafts.list(slug);
    if (loadId === draftsLoadId) drafts = result || [];
  } catch (err) {
    console.error('Failed to load drafts:', err);
    if (loadId === draftsLoadId) drafts = [];
  } finally {
    if (loadId === draftsLoadId) loadingDrafts = false;
  }
}

async function deleteDraft(requestTypeId) {
  const slug = context.getSlug();
  if (!slug || requestTypeId == null) return;
  try {
    await portal.drafts.delete(slug, requestTypeId);
    drafts = drafts.filter((draft) => draft.request_type_id !== requestTypeId);
  } catch (err) {
    if (err?.status !== 404) {
      console.error('Failed to delete draft:', err);
      errorToast(err?.message || 'Failed to delete draft');
      return;
    }
    drafts = drafts.filter((draft) => draft.request_type_id !== requestTypeId);
  }
}

function setShowDrafts(value) {
  if (value) {
    selectView('drafts');
    if (isAuthenticated()) void loadDrafts();
  } else {
    showDrafts = false;
  }
}

async function toggleDrafts() {
  context.closeProfileMenu();
  if (showDrafts) {
    showDrafts = false;
    navigate(`/portal/${context.getSlug()}`);
    return;
  }
  selectView('drafts');
  navigate(`/portal/${context.getSlug()}?view=drafts`);
  if (isAuthenticated()) await loadDrafts();
}

async function loadApprovals() {
  const slug = context.getSlug();
  if (!isAuthenticated() || !slug) return;
  const loadId = ++approvalsLoadId;
  loadingApprovals = true;
  try {
    const result = await portal.getMyApprovals(slug);
    if (loadId === approvalsLoadId) approvals = result || [];
  } catch (err) {
    console.error('Failed to load approvals:', err);
    if (loadId === approvalsLoadId) approvals = [];
  } finally {
    if (loadId === approvalsLoadId) loadingApprovals = false;
  }
}

async function loadAndViewApproval(approvalId, _replaceState = true) {
  const slug = context.getSlug();
  if (!slug) return;
  const loadId = ++approvalDetailLoadId;
  loadingApprovalDetail = true;
  try {
    const detail = await portal.getApproval(slug, approvalId);
    if (loadId !== approvalDetailLoadId) return;
    selectedApproval = detail;
    selectedApprovalRequest = null;
    if (detail?.item_id) {
      try {
        selectedApprovalRequest = await portal.getRequestDetail(slug, detail.item_id);
      } catch (err) {
        console.warn('Failed to load request context for approval:', err);
      }
    }
  } catch (err) {
    if (loadId !== approvalDetailLoadId) return;
    console.error('Failed to load approval:', err);
    errorToast(err?.message || 'Failed to load approval');
    selectedApproval = null;
    selectedApprovalRequest = null;
  } finally {
    if (loadId === approvalDetailLoadId) loadingApprovalDetail = false;
  }
}

async function viewApproval(approval) {
  selectedApproval = approval;
  navigate(`/portal/${context.getSlug()}?view=approvals&id=${approval.id}`);
  await loadAndViewApproval(approval.id, false);
}

async function decideApproval(decision) {
  const slug = context.getSlug();
  if (!selectedApproval || !slug) return;
  if (decision !== 'comment') {
    const ok = await confirm({
      message: `${decision === 'approve' ? 'Approve' : 'Reject'} this request?`,
      confirmText: decision === 'approve' ? 'Approve' : 'Reject',
    });
    if (!ok) return;
  }
  try {
    decidingApproval = true;
    await portal.decideApproval(slug, selectedApproval.id, decision, approvalComment);
    approvalComment = '';
    await loadAndViewApproval(selectedApproval.id);
    await loadApprovals();
  } catch (err) {
    console.error('Failed to decide approval:', err);
    errorToast(err?.message || 'Failed to record decision');
  } finally {
    decidingApproval = false;
  }
}

function closeApprovalDetail() {
  selectedApproval = null;
  selectedApprovalRequest = null;
  approvalComment = '';
  navigate(`/portal/${context.getSlug()}?view=approvals`);
}

function setShowApprovals(value) {
  if (value) {
    selectView('approvals');
    if (isAuthenticated()) void loadApprovals();
  } else {
    showApprovals = false;
    selectedApproval = null;
  }
}

async function toggleApprovals() {
  context.closeProfileMenu();
  if (showApprovals) {
    showApprovals = false;
    selectedApproval = null;
    navigate(`/portal/${context.getSlug()}`);
    return;
  }
  selectView('approvals');
  navigate(`/portal/${context.getSlug()}?view=approvals`);
  if (isAuthenticated()) await loadApprovals();
}

function hydrate(bootstrap) {
  if (!bootstrap?.authenticated) {
    requests = [];
    approvals = [];
    return;
  }
  requests = bootstrap.my_requests || [];
  approvals = bootstrap.my_approvals || [];
}

function reset() {
  requestsLoadId++;
  commentsLoadId++;
  draftsLoadId++;
  approvalsLoadId++;
  approvalDetailLoadId++;
  showRequests = false;
  requests = [];
  loadingRequests = false;
  selectedRequest = null;
  comments = [];
  loadingComments = false;
  newComment = '';
  addingComment = false;
  attachments = [];
  uploadingAttachment = false;
  showDrafts = false;
  drafts = [];
  loadingDrafts = false;
  showApprovals = false;
  approvals = [];
  loadingApprovals = false;
  selectedApproval = null;
  selectedApprovalRequest = null;
  loadingApprovalDetail = false;
  approvalComment = '';
  decidingApproval = false;
}

export const portalRequestsStore = {
  get visible() {
    return showRequests;
  },
  set visible(value) {
    showRequests = value;
  },
  get requests() {
    return requests;
  },
  get openCount() {
    return requests.filter((request) => !request.status_is_completed).length;
  },
  get loading() {
    return loadingRequests;
  },
  get selected() {
    return selectedRequest;
  },
  get comments() {
    return comments;
  },
  get loadingComments() {
    return loadingComments;
  },
  get newComment() {
    return newComment;
  },
  set newComment(value) {
    newComment = value;
  },
  get addingComment() {
    return addingComment;
  },
  get attachments() {
    return attachments;
  },
  get uploadingAttachment() {
    return uploadingAttachment;
  },
  load: loadRequests,
  view: viewRequest,
  loadComments,
  addComment,
  addAttachment,
  attachmentUrl: (attachmentId) => {
    const slug = context.getSlug();
    if (!slug || !selectedRequest) return '#';
    return portal.requestAttachmentUrl(slug, selectedRequest.id, attachmentId);
  },
  closeDetail: closeRequestDetail,
  toggle: toggleRequests,
  setVisible: setShowRequests,
  loadAndView: loadAndViewRequest,
};

export const portalDraftsStore = {
  get visible() {
    return showDrafts;
  },
  set visible(value) {
    showDrafts = value;
  },
  get drafts() {
    return drafts;
  },
  get loading() {
    return loadingDrafts;
  },
  get count() {
    return drafts.length;
  },
  load: loadDrafts,
  delete: deleteDraft,
  toggle: toggleDrafts,
  setVisible: setShowDrafts,
};

export const portalApprovalsStore = {
  get visible() {
    return showApprovals;
  },
  set visible(value) {
    showApprovals = value;
  },
  get approvals() {
    return approvals;
  },
  get pendingCount() {
    return approvals.filter((approval) => approval.status === 'pending').length;
  },
  get loading() {
    return loadingApprovals;
  },
  get selected() {
    return selectedApproval;
  },
  get selectedRequest() {
    return selectedApproval ? selectedApprovalRequest : null;
  },
  get loadingDetail() {
    return loadingApprovalDetail;
  },
  get comment() {
    return approvalComment;
  },
  set comment(value) {
    approvalComment = value;
  },
  get deciding() {
    return decidingApproval;
  },
  load: loadApprovals,
  view: viewApproval,
  loadAndView: loadAndViewApproval,
  decide: decideApproval,
  closeDetail: closeApprovalDetail,
  toggle: toggleApprovals,
  setVisible: setShowApprovals,
};

export const portalActivityStore = { hydrate, reset };
