import { useCallback, useEffect, useMemo, useState, type ReactNode } from "react";
import {
  Alert,
  Box,
  Button,
  Checkbox,
  Chip,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Divider,
  FormControl,
  FormControlLabel,
  IconButton,
  InputLabel,
  MenuItem,
  Pagination,
  Select,
  Slider,
  Switch,
  Tab,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Tabs,
  TextField,
  Tooltip,
  Typography,
} from "@mui/material";
import {
  DeleteOutlineRounded,
  EditOutlined,
  EmojiEventsRounded,
  GroupAddRounded,
  ImageRounded,
  LeaderboardRounded,
  RefreshRounded,
  TuneRounded,
} from "@mui/icons-material";
import { api, ApiError, imageVariant } from "../api/client";
import type { AdminFilter, ImageItem, TimeRange, User, Vote, VoteMode, VoteWinner } from "../api/types";
import { useAuth } from "../auth/AuthContext";
import { ConfirmDialog, useToast } from "../components/Feedback";
import { EmptyState, LoadingView } from "../components/Loading";
import { PageHeader, PageShell } from "../components/PageShell";
import { useStageImages } from "../theme/ThemeHost";
import { formatBytes, relativeTime } from "../utils/format";

const PAGE_SIZE = 25;
const TIME_OPTIONS: Array<{ value: TimeRange; label: string }> = [
  { value: "all", label: "All time" },
  { value: "day", label: "Last 24h" },
  { value: "week", label: "Last 7 days" },
  { value: "month", label: "Last 30 days" },
  { value: "year", label: "Last 365 days" },
  { value: "custom", label: "Custom seconds" },
];

const inputSx = { "& .MuiInputBase-input": { fontSize: "0.86rem" } } as const;

export function AdminPage() {
  const { user } = useAuth();
  const { notify } = useToast();
  const [tab, setTab] = useState<"votes" | "images" | "users">("votes");

  const [modes, setModes] = useState<VoteMode[]>([]);
  const [votes, setVotes] = useState<Vote[]>([]);
  const [votesTotal, setVotesTotal] = useState(0);
  const [votesLoading, setVotesLoading] = useState(true);
  const [votePage, setVotePage] = useState(1);
  const [voteFilters, setVoteFilters] = useState<AdminFilter>({ time: "all" });
  const [voteDraft, setVoteDraft] = useState<AdminFilter>({ time: "all" });
  const [votesRefresh, setVotesRefresh] = useState(0);

  const [images, setImages] = useState<ImageItem[]>([]);
  const [imagesTotal, setImagesTotal] = useState(0);
  const [imagesLoading, setImagesLoading] = useState(false);
  const [imagePage, setImagePage] = useState(1);
  const [imageFilters, setImageFilters] = useState<AdminFilter>({});
  const [imageDraft, setImageDraft] = useState<AdminFilter>({});
  const [imagesRefresh, setImagesRefresh] = useState(0);

  const [users, setUsers] = useState<User[]>([]);
  const [usersLoading, setUsersLoading] = useState(false);
  const [usersRefresh, setUsersRefresh] = useState(0);

  const [editVote, setEditVote] = useState<Vote | null>(null);
  const [editImage, setEditImage] = useState<ImageItem | null>(null);
  const [editUser, setEditUser] = useState<User | null>(null);
  const [createUserOpen, setCreateUserOpen] = useState(false);
  const [modesOpen, setModesOpen] = useState(false);
  const [modeDraft, setModeDraft] = useState<VoteMode | null>(null);

  const [deleteVote, setDeleteVote] = useState<Vote | null>(null);
  const [deleteImage, setDeleteImage] = useState<ImageItem | null>(null);
  const [deleteUser, setDeleteUser] = useState<User | null>(null);
  const [deleteMode, setDeleteMode] = useState<VoteMode | null>(null);
  const [bulkVotesOpen, setBulkVotesOpen] = useState(false);
  const [bulkImagesOpen, setBulkImagesOpen] = useState(false);
  const [keepVotes, setKeepVotes] = useState(false);
  const [working, setWorking] = useState(false);

  const loadModes = useCallback(async () => {
    try {
      setModes(await api.modes());
    } catch {
      // mode list is informative only
    }
  }, []);

  useEffect(() => {
    void loadModes();
  }, [loadModes]);

  const loadVotes = useCallback(async () => {
    setVotesLoading(true);
    try {
      const result = await api.admin.listVotes({
        ...voteFilters,
        amount: PAGE_SIZE,
        offset: (votePage - 1) * PAGE_SIZE,
      });
      setVotes(result.items);
      setVotesTotal(result.total);
    } catch (error) {
      notify(error instanceof ApiError ? error.message : "Could not load votes.", "error");
    } finally {
      setVotesLoading(false);
    }
  }, [notify, voteFilters, votePage]);

  useEffect(() => {
    if (tab === "votes") void loadVotes();
  }, [loadVotes, tab, votesRefresh]);

  const loadImages = useCallback(async () => {
    setImagesLoading(true);
    try {
      const result = await api.admin.listImages({
        ...imageFilters,
        amount: PAGE_SIZE,
        offset: (imagePage - 1) * PAGE_SIZE,
      });
      setImages(result.items);
      setImagesTotal(result.total);
    } catch (error) {
      notify(error instanceof ApiError ? error.message : "Could not load images.", "error");
    } finally {
      setImagesLoading(false);
    }
  }, [imageFilters, imagePage, notify]);

  useEffect(() => {
    if (tab === "images") void loadImages();
  }, [imagesRefresh, loadImages, tab]);

  const loadUsers = useCallback(async () => {
    setUsersLoading(true);
    try {
      const result = await api.admin.listUsers();
      setUsers(result.items);
    } catch (error) {
      notify(error instanceof ApiError ? error.message : "Could not load users.", "error");
    } finally {
      setUsersLoading(false);
    }
  }, [notify]);

  useEffect(() => {
    if (tab === "users") void loadUsers();
  }, [loadUsers, tab, usersRefresh]);

  const voteStage = useMemo(
    () =>
      votes.flatMap((vote) =>
        [vote.leftImage?.url, vote.rightImage?.url].filter((url): url is string => Boolean(url)),
      ),
    [votes],
  );
  const imageStage = useMemo(() => images.map((image) => image.url), [images]);
  useStageImages(tab === "votes" ? voteStage : tab === "images" ? imageStage : []);

  const applyVoteFilters = () => {
    if (voteDraft.time === "custom" && (!voteDraft.seconds || voteDraft.seconds <= 0)) {
      notify("Custom ranges need a positive number of seconds.", "warning");
      return;
    }
    setVotePage(1);
    setVoteFilters({ ...voteDraft });
  };

  const resetVoteFilters = () => {
    setVoteDraft({ time: "all" });
    setVoteFilters({ time: "all" });
    setVotePage(1);
  };

  const applyImageFilters = () => {
    setImagePage(1);
    setImageFilters({ ...imageDraft });
  };

  const resetImageFilters = () => {
    setImageDraft({});
    setImageFilters({});
    setImagePage(1);
  };

  const saveVote = async (payload: {
    mode: string;
    winner?: VoteWinner;
    value?: number;
    minValue?: number;
    maxValue?: number;
    metadata?: Record<string, unknown>;
  }) => {
    if (!editVote) return;
    setWorking(true);
    try {
      await api.admin.updateVote(editVote.id, payload);
      notify("Vote updated.", "success");
      setEditVote(null);
      setVotesRefresh((value) => value + 1);
    } catch (error) {
      notify(error instanceof ApiError ? error.message : "Could not update the vote.", "error");
    } finally {
      setWorking(false);
    }
  };

  const saveImage = async (values: { author: string; title: string }) => {
    if (!editImage) return;
    setWorking(true);
    try {
      await api.admin.updateImage(editImage.id, values);
      notify("Image details updated.", "success");
      setEditImage(null);
      setImagesRefresh((value) => value + 1);
    } catch (error) {
      notify(error instanceof ApiError ? error.message : "Could not update the image.", "error");
    } finally {
      setWorking(false);
    }
  };

  const saveUser = async (
    target: User,
    values: { displayName?: string; password?: string; isAdmin?: boolean },
  ) => {
    setWorking(true);
    try {
      await api.admin.updateUser(target.id, values);
      notify("User updated.", "success");
      setEditUser(null);
      setUsersRefresh((value) => value + 1);
    } catch (error) {
      notify(error instanceof ApiError ? error.message : "Could not update the user.", "error");
    } finally {
      setWorking(false);
    }
  };

  const createUser = async (values: {
    username: string;
    password: string;
    displayName: string;
    isAdmin: boolean;
  }) => {
    setWorking(true);
    try {
      await api.admin.createUser(values);
      notify("Account created.", "success");
      setCreateUserOpen(false);
      setUsersRefresh((value) => value + 1);
    } catch (error) {
      notify(error instanceof ApiError ? error.message : "Could not create the account.", "error");
    } finally {
      setWorking(false);
    }
  };

  const confirmDeleteVote = async () => {
    if (!deleteVote) return;
    setWorking(true);
    try {
      await api.admin.deleteVote(deleteVote.id);
      notify("Vote deleted.", "success");
      setDeleteVote(null);
      setVotesRefresh((value) => value + 1);
    } catch (error) {
      notify(error instanceof ApiError ? error.message : "Could not delete the vote.", "error");
    } finally {
      setWorking(false);
    }
  };

  const confirmDeleteImage = async () => {
    if (!deleteImage) return;
    setWorking(true);
    try {
      await api.admin.deleteImage(deleteImage.id, keepVotes);
      notify("Image deleted.", "success");
      setDeleteImage(null);
      setImagesRefresh((value) => value + 1);
      setVotesRefresh((value) => value + 1);
    } catch (error) {
      notify(error instanceof ApiError ? error.message : "Could not delete the image.", "error");
    } finally {
      setWorking(false);
    }
  };

  const confirmDeleteUser = async () => {
    if (!deleteUser) return;
    setWorking(true);
    try {
      await api.admin.deleteUser(deleteUser.id);
      notify("Account deleted.", "success");
      setDeleteUser(null);
      setUsersRefresh((value) => value + 1);
    } catch (error) {
      notify(error instanceof ApiError ? error.message : "Could not delete the account.", "error");
    } finally {
      setWorking(false);
    }
  };

  const confirmBulkVotes = async () => {
    setWorking(true);
    try {
      const result = await api.admin.bulkDeleteVotes(voteFilters);
      notify(`${result.deleted} votes deleted.`, "success");
      setBulkVotesOpen(false);
      setVotesRefresh((value) => value + 1);
    } catch (error) {
      notify(error instanceof ApiError ? error.message : "Could not delete the votes.", "error");
    } finally {
      setWorking(false);
    }
  };

  const confirmBulkImages = async () => {
    setWorking(true);
    try {
      const result = await api.admin.bulkDeleteImages({ ...imageFilters, keepVotes });
      notify(`${result.deleted} images deleted.`, "success");
      setBulkImagesOpen(false);
      setImagesRefresh((value) => value + 1);
      setVotesRefresh((value) => value + 1);
    } catch (error) {
      notify(error instanceof ApiError ? error.message : "Could not delete the images.", "error");
    } finally {
      setWorking(false);
    }
  };

  const saveMode = async (draft: {
    id?: number;
    name: string;
    description: string;
    minValue: number;
    maxValue: number;
    isBinary: boolean;
  }) => {
    setWorking(true);
    try {
      if (draft.id) {
        await api.admin.updateMode(draft.id, {
          name: draft.name,
          description: draft.description,
          minValue: draft.minValue,
          maxValue: draft.maxValue,
          isBinary: draft.isBinary,
        });
      } else {
        await api.admin.createMode({
          name: draft.name,
          description: draft.description,
          minValue: draft.minValue,
          maxValue: draft.maxValue,
          isBinary: draft.isBinary,
        });
      }
      notify("Mode saved.", "success");
      setModeDraft(null);
      await loadModes();
    } catch (error) {
      notify(error instanceof ApiError ? error.message : "Could not save the mode.", "error");
    } finally {
      setWorking(false);
    }
  };

  const confirmDeleteMode = async () => {
    if (!deleteMode) return;
    setWorking(true);
    try {
      await api.admin.deleteMode(deleteMode.id);
      notify("Mode deleted.", "success");
      setDeleteMode(null);
      await loadModes();
    } catch (error) {
      notify(error instanceof ApiError ? error.message : "Could not delete the mode.", "error");
    } finally {
      setWorking(false);
    }
  };

  if (!user?.isAdmin) {
    return (
      <PageShell>
        <EmptyState
          emoji="🔐"
          title="Admins only"
          description="This room is for the curator. Ask an administrator for access."
        />
      </PageShell>
    );
  }

  return (
    <PageShell maxWidth="xl">
      <PageHeader
        eyebrow="curator's room"
        title="Tend the dream."
        description="Rankings, images and people — editable while the paint is still wet."
        action={
          <>
            <Button
              variant="outlined"
              size="small"
              startIcon={<TuneRounded />}
              onClick={() => setModesOpen(true)}
              sx={{ borderRadius: 99, textTransform: "none" }}
            >
              Modes
            </Button>
            <Button
              variant="outlined"
              size="small"
              startIcon={<RefreshRounded />}
              onClick={() => {
                setVotesRefresh((value) => value + 1);
                setImagesRefresh((value) => value + 1);
                setUsersRefresh((value) => value + 1);
              }}
              sx={{ borderRadius: 99, textTransform: "none" }}
            >
              Refresh
            </Button>
          </>
        }
      />

      <Tabs
        value={tab}
        onChange={(_event, value: "votes" | "images" | "users") => setTab(value)}
        sx={{
          mb: 3,
          minHeight: 42,
          "& .MuiTab-root": { textTransform: "none", fontWeight: 650, minHeight: 42 },
        }}
      >
        <Tab value="votes" label="Rankings" icon={<LeaderboardRounded fontSize="small" />} iconPosition="start" />
        <Tab value="images" label="Images" icon={<ImageRounded fontSize="small" />} iconPosition="start" />
        <Tab value="users" label="People" icon={<GroupAddRounded fontSize="small" />} iconPosition="start" />
      </Tabs>

      {tab === "votes" && (
        <Box>
          <FilterCard>
            <TimeFilter draft={voteDraft} setDraft={setVoteDraft} />
            <TextField
              label="Voter username"
              size="small"
              value={voteDraft.user ?? ""}
              onChange={(event) => setVoteDraft({ ...voteDraft, user: event.target.value })}
              sx={{ minWidth: 150, ...inputSx }}
            />
            <TextField
              label="Image author"
              size="small"
              value={voteDraft.author ?? ""}
              onChange={(event) => setVoteDraft({ ...voteDraft, author: event.target.value })}
              sx={{ minWidth: 150, ...inputSx }}
            />
            <TextField
              label="Title regex"
              size="small"
              value={voteDraft.titleRegex ?? ""}
              onChange={(event) => setVoteDraft({ ...voteDraft, titleRegex: event.target.value })}
              sx={{ minWidth: 150, ...inputSx }}
              inputProps={{ spellCheck: false }}
            />
            <FormControl size="small" sx={{ minWidth: 130 }}>
              <InputLabel>Mode</InputLabel>
              <Select
                label="Mode"
                value={voteDraft.mode ?? ""}
                onChange={(event) => setVoteDraft({ ...voteDraft, mode: event.target.value })}
              >
                <MenuItem value="">Any mode</MenuItem>
                {modes.map((mode) => (
                  <MenuItem key={mode.id} value={mode.name}>
                    {mode.name}
                  </MenuItem>
                ))}
              </Select>
            </FormControl>
            <Box sx={{ flex: 1 }} />
            <Button onClick={applyVoteFilters} variant="contained" sx={{ borderRadius: 99, textTransform: "none" }}>
              Apply
            </Button>
            <Button onClick={resetVoteFilters} sx={{ textTransform: "none" }}>
              Reset
            </Button>
            <Button
              onClick={() => setBulkVotesOpen(true)}
              color="error"
              variant="outlined"
              startIcon={<DeleteOutlineRounded />}
              sx={{ borderRadius: 99, textTransform: "none" }}
            >
              Delete matches
            </Button>
          </FilterCard>

          <Typography variant="body2" color="text.secondary" sx={{ mb: 1.4, opacity: 0.8 }}>
            {votesTotal} vote{votesTotal === 1 ? "" : "s"} matching these filters.
          </Typography>

          {votesLoading ? (
            <LoadingView label="Tallying the verdicts…" />
          ) : votes.length === 0 ? (
            <EmptyState emoji="🕳️" title="No votes here" description="Loosen the filters or start a duel." />
          ) : (
            <TableContainer
              className="glass"
              sx={{ borderRadius: 4, overflowX: "auto", border: "1px solid rgba(255,255,255,.08)" }}
            >
              <Table size="small" sx={{ minWidth: 860 }}>
                <TableHead>
                  <TableRow>
                    <TableCell>Vote</TableCell>
                    <TableCell>Pair</TableCell>
                    <TableCell>Outcome</TableCell>
                    <TableCell>Voter</TableCell>
                    <TableCell>When</TableCell>
                    <TableCell align="right">Actions</TableCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {votes.map((vote) => (
                    <TableRow key={vote.id} hover>
                      <TableCell>
                        <Typography sx={{ fontWeight: 700 }}>#{vote.id}</Typography>
                        <Typography variant="caption" color="text.secondary">
                          {vote.modeName}
                        </Typography>
                      </TableCell>
                      <TableCell>
                        <Box className="admin-thumb-pair">
                          {vote.leftImage && (
                            <Tooltip title={`left: ${vote.leftImage.title || vote.leftImage.author}`}>
                              <Box
                                component="img"
                                className="admin-thumb"
                                src={imageVariant(vote.leftImage.url, { w: 160, quality: 70 })}
                                alt=""
                              />
                            </Tooltip>
                          )}
                          <Typography variant="caption" sx={{ mx: 0.4, opacity: 0.5 }}>
                            vs
                          </Typography>
                          {vote.rightImage && (
                            <Tooltip title={`right: ${vote.rightImage.title || vote.rightImage.author}`}>
                              <Box
                                component="img"
                                className="admin-thumb"
                                src={imageVariant(vote.rightImage.url, { w: 160, quality: 70 })}
                                alt=""
                              />
                            </Tooltip>
                          )}
                        </Box>
                      </TableCell>
                      <TableCell>
                        {vote.winner === "tie" ? (
                          <Chip size="small" label="draw" />
                        ) : (
                          <Chip
                            size="small"
                            icon={<EmojiEventsRounded sx={{ fontSize: 14 }} />}
                            label={vote.winner}
                            color={vote.winner === "left" ? "primary" : "secondary"}
                            variant="outlined"
                          />
                        )}
                        {vote.value !== undefined && (
                          <Typography variant="caption" display="block" color="text.secondary">
                            {vote.value} / {vote.minValue}–{vote.maxValue} · weight {vote.weight.toFixed(2)}
                          </Typography>
                        )}
                      </TableCell>
                      <TableCell>
                        <Typography variant="body2">{vote.username || "—"}</Typography>
                        {vote.user && (
                          <Typography variant="caption" color="text.secondary">
                            id {vote.user.id}
                          </Typography>
                        )}
                      </TableCell>
                      <TableCell>
                        <Typography variant="body2">{relativeTime(vote.createdAt)}</Typography>
                      </TableCell>
                      <TableCell align="right">
                        <Tooltip title="Edit outcome">
                          <IconButton size="small" onClick={() => setEditVote(vote)}>
                            <EditOutlined fontSize="small" />
                          </IconButton>
                        </Tooltip>
                        <Tooltip title="Delete vote">
                          <IconButton size="small" color="error" onClick={() => setDeleteVote(vote)}>
                            <DeleteOutlineRounded fontSize="small" />
                          </IconButton>
                        </Tooltip>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </TableContainer>
          )}

          <Pager
            page={votePage}
            total={votesTotal}
            onChange={(page) => setVotePage(page)}
          />
        </Box>
      )}

      {tab === "images" && (
        <Box>
          <FilterCard>
            <TextField
              label="Author contains"
              size="small"
              value={imageDraft.author ?? ""}
              onChange={(event) => setImageDraft({ ...imageDraft, author: event.target.value })}
              sx={{ minWidth: 180, ...inputSx }}
            />
            <TextField
              label="Title regex"
              size="small"
              value={imageDraft.titleRegex ?? ""}
              onChange={(event) => setImageDraft({ ...imageDraft, titleRegex: event.target.value })}
              sx={{ minWidth: 180, ...inputSx }}
              inputProps={{ spellCheck: false }}
            />
            <FormControlLabel
              control={
                <Checkbox
                  checked={imageDraft.time === "all"}
                  onChange={(event) =>
                    setImageDraft({ ...imageDraft, time: event.target.checked ? "all" : undefined })
                  }
                />
              }
              label="all uploads"
            />
            <Box sx={{ flex: 1 }} />
            <Button onClick={applyImageFilters} variant="contained" sx={{ borderRadius: 99, textTransform: "none" }}>
              Apply
            </Button>
            <Button onClick={resetImageFilters} sx={{ textTransform: "none" }}>
              Reset
            </Button>
            <Button
              onClick={() => setBulkImagesOpen(true)}
              color="error"
              variant="outlined"
              startIcon={<DeleteOutlineRounded />}
              sx={{ borderRadius: 99, textTransform: "none" }}
            >
              Delete matches
            </Button>
          </FilterCard>

          <Typography variant="body2" color="text.secondary" sx={{ mb: 1.4, opacity: 0.8 }}>
            {imagesTotal} image{imagesTotal === 1 ? "" : "s"} matching these filters.
          </Typography>

          {imagesLoading ? (
            <LoadingView label="Dusting off the frames…" />
          ) : images.length === 0 ? (
            <EmptyState emoji="🖼️" title="No images found" description="Change the filter or upload art." />
          ) : (
            <Box
              sx={{
                display: "grid",
                gridTemplateColumns: {
                  xs: "1fr",
                  sm: "repeat(2, minmax(0, 1fr))",
                  md: "repeat(3, minmax(0, 1fr))",
                  lg: "repeat(4, minmax(0, 1fr))",
                },
                gap: 2,
              }}
            >
              {images.map((image) => (
                <Box key={image.id} className="glass" sx={{ borderRadius: 4, overflow: "hidden" }}>
                  <Box sx={{ position: "relative", aspectRatio: "4 / 3", bgcolor: "rgba(0,0,0,.25)" }}>
                    <Box
                      component="img"
                      src={imageVariant(image.url, { w: 600, quality: 78 })}
                      alt={image.title || image.author}
                      loading="lazy"
                      sx={{ width: "100%", height: "100%", objectFit: "cover" }}
                    />
                  </Box>
                  <Box sx={{ p: 1.5 }}>
                    <Typography noWrap className="display" sx={{ fontSize: "1.1rem", fontWeight: 620 }}>
                      {image.title || "Untitled"}
                    </Typography>
                    <Typography noWrap variant="body2" color="text.secondary">
                      {image.author} · {image.width} × {image.height} · {formatBytes(image.sizeBytes)}
                    </Typography>
                    <Typography variant="caption" color="text.secondary" component="div" noWrap>
                      {image.id} · uploaded {relativeTime(image.createdAt)}
                      {image.uploadedBy ? ` · user ${image.uploadedBy}` : " · anonymous"}
                    </Typography>
                    <Box sx={{ display: "flex", justifyContent: "flex-end", mt: 0.6 }}>
                      <Button size="small" startIcon={<EditOutlined />} onClick={() => setEditImage(image)}>
                        Edit
                      </Button>
                      <Button
                        size="small"
                        color="error"
                        startIcon={<DeleteOutlineRounded />}
                        onClick={() => setDeleteImage(image)}
                      >
                        Delete
                      </Button>
                    </Box>
                  </Box>
                </Box>
              ))}
            </Box>
          )}

          <Pager page={imagePage} total={imagesTotal} onChange={(page) => setImagePage(page)} />
        </Box>
      )}

      {tab === "users" && (
        <Box>
          <FilterCard>
            <Box sx={{ flex: 1 }}>
              <Typography sx={{ fontWeight: 700 }}>
                {users.length} account{users.length === 1 ? "" : "s"}
              </Typography>
              <Typography variant="body2" color="text.secondary">
                Admins can manage everything; regular users can submit and vote.
              </Typography>
            </Box>
            <Button
              variant="contained"
              startIcon={<GroupAddRounded />}
              onClick={() => setCreateUserOpen(true)}
              sx={{ borderRadius: 99, textTransform: "none" }}
            >
              New account
            </Button>
          </FilterCard>

          {usersLoading ? (
            <LoadingView label="Gathering the dreamers…" />
          ) : (
            <TableContainer
              className="glass"
              sx={{ borderRadius: 4, overflowX: "auto", border: "1px solid rgba(255,255,255,.08)" }}
            >
              <Table size="small" sx={{ minWidth: 700 }}>
                <TableHead>
                  <TableRow>
                    <TableCell>Account</TableCell>
                    <TableCell>Display name</TableCell>
                    <TableCell>Created</TableCell>
                    <TableCell>Admin</TableCell>
                    <TableCell align="right">Actions</TableCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {users.map((entry) => (
                    <TableRow key={entry.id} hover>
                      <TableCell>
                        <Typography sx={{ fontWeight: 700 }}>@{entry.username}</Typography>
                        <Typography variant="caption" color="text.secondary">
                          id {entry.id}
                        </Typography>
                      </TableCell>
                      <TableCell>{entry.displayName}</TableCell>
                      <TableCell>{relativeTime(entry.createdAt)}</TableCell>
                      <TableCell>
                        <Switch
                          size="small"
                          checked={entry.isAdmin}
                          disabled={entry.id === user.id && entry.isAdmin}
                          onChange={async (event) => {
                            try {
                              await api.admin.updateUser(entry.id, { isAdmin: event.target.checked });
                              notify("Permissions updated.", "success");
                              setUsersRefresh((value) => value + 1);
                            } catch (error) {
                              notify(
                                error instanceof ApiError ? error.message : "Could not update permissions.",
                                "error",
                              );
                            }
                          }}
                        />
                      </TableCell>
                      <TableCell align="right">
                        <Tooltip title="Edit account">
                          <IconButton size="small" onClick={() => setEditUser(entry)}>
                            <EditOutlined fontSize="small" />
                          </IconButton>
                        </Tooltip>
                        <Tooltip title={entry.id === user.id ? "You cannot delete yourself" : "Delete account"}>
                          <span>
                            <IconButton
                              size="small"
                              color="error"
                              disabled={entry.id === user.id}
                              onClick={() => setDeleteUser(entry)}
                            >
                              <DeleteOutlineRounded fontSize="small" />
                            </IconButton>
                          </span>
                        </Tooltip>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </TableContainer>
          )}
        </Box>
      )}

      <VoteEditDialog
        vote={editVote}
        modes={modes}
        working={working}
        onClose={() => setEditVote(null)}
        onSave={saveVote}
      />

      <ImageEditDialog
        image={editImage}
        working={working}
        onClose={() => setEditImage(null)}
        onSave={saveImage}
      />

      <UserEditDialog
        user={editUser}
        working={working}
        onClose={() => setEditUser(null)}
        onSave={saveUser}
      />

      <CreateUserDialog
        open={createUserOpen}
        working={working}
        onClose={() => setCreateUserOpen(false)}
        onSave={createUser}
      />

      <ModesDialog
        open={modesOpen}
        modes={modes}
        draft={modeDraft}
        working={working}
        onClose={() => {
          setModesOpen(false);
          setModeDraft(null);
        }}
        onDraft={setModeDraft}
        onSave={saveMode}
        onDelete={(mode) => setDeleteMode(mode)}
      />

      <ConfirmDialog
        open={Boolean(deleteVote)}
        title="Delete this vote?"
        description={
          <>
            Vote #{deleteVote?.id} by {deleteVote?.username} will disappear from the rankings.
          </>
        }
        confirmLabel="Delete vote"
        danger
        loading={working}
        onCancel={() => setDeleteVote(null)}
        onConfirm={confirmDeleteVote}
      />

      <ConfirmDialog
        open={Boolean(deleteImage)}
        title="Delete this image?"
        description={
          <Box>
            <Typography variant="body2">
              “{deleteImage?.title || "Untitled"}” by {deleteImage?.author} will be removed from disk.
            </Typography>
            <FormControlLabel
              sx={{ mt: 1 }}
              control={<Checkbox checked={keepVotes} onChange={(event) => setKeepVotes(event.target.checked)} />}
              label="Keep its vote history"
            />
          </Box>
        }
        confirmLabel="Delete image"
        danger
        loading={working}
        onCancel={() => setDeleteImage(null)}
        onConfirm={confirmDeleteImage}
      />

      <ConfirmDialog
        open={Boolean(deleteUser)}
        title="Delete this account?"
        description={
          <>
            @{deleteUser?.username} will lose access. Their submitted images stay online, but the
            account link is removed.
          </>
        }
        confirmLabel="Delete account"
        danger
        loading={working}
        onCancel={() => setDeleteUser(null)}
        onConfirm={confirmDeleteUser}
      />

      <ConfirmDialog
        open={Boolean(deleteMode)}
        title="Delete this mode?"
        description={`The “${deleteMode?.name}” voting scale will be removed. Modes with votes cannot be deleted.`}
        confirmLabel="Delete mode"
        danger
        loading={working}
        onCancel={() => setDeleteMode(null)}
        onConfirm={confirmDeleteMode}
      />

      <ConfirmDialog
        open={bulkVotesOpen}
        title="Delete every matching vote?"
        description={
          <Box>
            <Typography variant="body2" sx={{ mb: 1 }}>
              {votesTotal} votes will be removed permanently. Filters:{" "}
              {describeFilters(voteFilters)}.
            </Typography>
            <Alert severity="warning" sx={{ borderRadius: 2 }}>
              This cannot be undone.
            </Alert>
          </Box>
        }
        confirmLabel={`Delete ${votesTotal} votes`}
        danger
        loading={working}
        onCancel={() => setBulkVotesOpen(false)}
        onConfirm={confirmBulkVotes}
      />

      <ConfirmDialog
        open={bulkImagesOpen}
        title="Delete every matching image?"
        description={
          <Box>
            <Typography variant="body2" sx={{ mb: 1 }}>
              {imagesTotal} images will be removed from disk. Filters: {describeFilters(imageFilters)}.
            </Typography>
            <FormControlLabel
              control={<Checkbox checked={keepVotes} onChange={(event) => setKeepVotes(event.target.checked)} />}
              label="Keep vote history"
            />
            <Alert severity="warning" sx={{ borderRadius: 2, mt: 1 }}>
              This cannot be undone.
            </Alert>
          </Box>
        }
        confirmLabel={`Delete ${imagesTotal} images`}
        danger
        loading={working}
        onCancel={() => setBulkImagesOpen(false)}
        onConfirm={confirmBulkImages}
      />
    </PageShell>
  );
}

function describeFilters(filter: AdminFilter): string {
  const parts: string[] = [];
  if (filter.time) parts.push(`time=${filter.time}${filter.seconds ? ` (${filter.seconds}s)` : ""}`);
  if (filter.user) parts.push(`user=${filter.user}`);
  if (filter.author) parts.push(`author=${filter.author}`);
  if (filter.titleRegex) parts.push(`title~/${filter.titleRegex}/`);
  if (filter.mode) parts.push(`mode=${filter.mode}`);
  return parts.length > 0 ? parts.join(", ") : "none";
}

function FilterCard({ children }: { children: ReactNode }) {
  return (
    <Box
      className="glass"
      sx={{
        borderRadius: 4,
        p: 1.6,
        mb: 2,
        display: "flex",
        gap: 1.2,
        alignItems: "center",
        flexWrap: "wrap",
      }}
    >
      {children}
    </Box>
  );
}

function TimeFilter({
  draft,
  setDraft,
}: {
  draft: AdminFilter;
  setDraft: (filter: AdminFilter) => void;
}) {
  return (
    <>
      <FormControl size="small" sx={{ minWidth: 145 }}>
        <InputLabel>Time</InputLabel>
        <Select
          label="Time"
          value={draft.time ?? "all"}
          onChange={(event) => {
            const time = event.target.value as TimeRange;
            setDraft({
              ...draft,
              time,
              seconds: time === "custom" ? (draft.seconds ?? 86400) : undefined,
            });
          }}
        >
          {TIME_OPTIONS.map((option) => (
            <MenuItem key={option.value} value={option.value}>
              {option.label}
            </MenuItem>
          ))}
        </Select>
      </FormControl>
      {draft.time === "custom" && (
        <TextField
          label="Seconds"
          type="number"
          size="small"
          value={draft.seconds ?? 86400}
          onChange={(event) => setDraft({ ...draft, seconds: Number(event.target.value) })}
          sx={{ width: 110, ...inputSx }}
        />
      )}
    </>
  );
}

function Pager({
  page,
  total,
  onChange,
}: {
  page: number;
  total: number;
  onChange: (page: number) => void;
}) {
  const pages = Math.max(1, Math.ceil(total / PAGE_SIZE));
  if (total <= PAGE_SIZE) return null;
  return (
    <Box sx={{ display: "flex", justifyContent: "center", mt: 3 }}>
      <Pagination count={pages} page={page} onChange={(_event, value) => onChange(value)} shape="rounded" />
    </Box>
  );
}

function VoteEditDialog({
  vote,
  modes,
  working,
  onClose,
  onSave,
}: {
  vote: Vote | null;
  modes: VoteMode[];
  working: boolean;
  onClose: () => void;
  onSave: (payload: {
    mode: string;
    winner?: VoteWinner;
    value?: number;
    minValue?: number;
    maxValue?: number;
    metadata?: Record<string, unknown>;
  }) => void;
}) {
  const [modeName, setModeName] = useState("");
  const [winner, setWinner] = useState<VoteWinner>("tie");
  const [value, setValue] = useState(50);
  const [metadataText, setMetadataText] = useState("{}");
  const [error, setError] = useState("");

  useEffect(() => {
    if (!vote) return;
    const selected = modes.find((mode) => mode.name === vote.modeName);
    setModeName(vote.modeName);
    setWinner(vote.winner);
    setValue(vote.value ?? Math.round(((selected?.minValue ?? vote.minValue) + (selected?.maxValue ?? vote.maxValue)) / 2));
    setMetadataText(JSON.stringify(vote.metadata ?? {}, null, 2));
    setError("");
  }, [modes, vote]);

  const selected = modes.find((mode) => mode.name === modeName);
  const binary = selected?.isBinary ?? false;
  const min = selected?.minValue ?? vote?.minValue ?? 1;
  const max = selected?.maxValue ?? vote?.maxValue ?? 5;

  const save = () => {
    let metadata: Record<string, unknown> = {};
    try {
      const parsed = JSON.parse(metadataText || "{}") as unknown;
      if (parsed && typeof parsed === "object" && !Array.isArray(parsed)) {
        metadata = parsed as Record<string, unknown>;
      } else {
        throw new Error("not an object");
      }
    } catch {
      setError("Metadata has to be a valid JSON object.");
      return;
    }
    setError("");
    onSave(
      binary
        ? { mode: modeName, winner, metadata }
        : { mode: modeName, value, minValue: min, maxValue: max, metadata },
    );
  };

  return (
    <Dialog open={Boolean(vote)} onClose={working ? undefined : onClose} maxWidth="sm" fullWidth>
      <DialogTitle sx={{ fontFamily: "var(--ad-font-display)", fontWeight: 650 }}>
        Reshape vote #{vote?.id}
      </DialogTitle>
      <DialogContent>
        <Box sx={{ display: "grid", gap: 2, mt: 0.5 }}>
          <Box sx={{ display: "flex", gap: 1 }}>
            {vote?.leftImage && (
              <Box sx={{ flex: 1 }}>
                <Box
                  component="img"
                  src={imageVariant(vote.leftImage.url, { w: 400, quality: 80 })}
                  alt=""
                  sx={{ width: "100%", height: 110, objectFit: "cover", borderRadius: 2 }}
                />
                <Typography variant="caption" noWrap component="div">
                  left · {vote.leftImage.title || vote.leftImage.author}
                </Typography>
              </Box>
            )}
            {vote?.rightImage && (
              <Box sx={{ flex: 1 }}>
                <Box
                  component="img"
                  src={imageVariant(vote.rightImage.url, { w: 400, quality: 80 })}
                  alt=""
                  sx={{ width: "100%", height: 110, objectFit: "cover", borderRadius: 2 }}
                />
                <Typography variant="caption" noWrap component="div">
                  right · {vote.rightImage.title || vote.rightImage.author}
                </Typography>
              </Box>
            )}
          </Box>

          <FormControl size="small" fullWidth>
            <InputLabel>Voting mode</InputLabel>
            <Select
              label="Voting mode"
              value={modeName}
              onChange={(event) => {
                const next = modes.find((mode) => mode.name === event.target.value);
                setModeName(event.target.value);
                if (next && !next.isBinary) {
                  setValue(Math.round((next.minValue + next.maxValue) / 2));
                }
              }}
            >
              {modes.map((mode) => (
                <MenuItem key={mode.id} value={mode.name}>
                  {mode.name} {mode.isBinary ? "(duel)" : `(${mode.minValue}–${mode.maxValue})`}
                </MenuItem>
              ))}
            </Select>
          </FormControl>

          {binary ? (
            <FormControl size="small" fullWidth>
              <InputLabel>Winner</InputLabel>
              <Select
                label="Winner"
                value={winner}
                onChange={(event) => setWinner(event.target.value as VoteWinner)}
              >
                <MenuItem value="left">left image</MenuItem>
                <MenuItem value="right">right image</MenuItem>
                <MenuItem value="tie">draw</MenuItem>
              </Select>
            </FormControl>
          ) : (
            <Box>
              <Typography variant="body2" color="text.secondary" sx={{ mb: 0.5 }}>
                Value: <strong>{value}</strong> / {min}–{max}
              </Typography>
              <Slider
                value={value}
                onChange={(_event, next) => setValue(next as number)}
                min={min}
                max={max}
                step={1}
                marks={[
                  { value: min, label: `${min}` },
                  { value: Math.round((min + max) / 2), label: "even" },
                  { value: max, label: `${max}` },
                ]}
              />
            </Box>
          )}

          <TextField
            label="Metadata JSON"
            value={metadataText}
            onChange={(event) => setMetadataText(event.target.value)}
            multiline
            minRows={4}
            fullWidth
            InputProps={{ sx: { fontFamily: "monospace", fontSize: "0.82rem" } }}
          />

          {error && <Alert severity="warning">{error}</Alert>}
        </Box>
      </DialogContent>
      <DialogActions sx={{ px: 3, pb: 2.5 }}>
        <Button onClick={onClose} disabled={working} color="inherit">
          Cancel
        </Button>
        <Button onClick={save} disabled={working} variant="contained">
          {working ? "Saving…" : "Save vote"}
        </Button>
      </DialogActions>
    </Dialog>
  );
}

function ImageEditDialog({
  image,
  working,
  onClose,
  onSave,
}: {
  image: ImageItem | null;
  working: boolean;
  onClose: () => void;
  onSave: (values: { author: string; title: string }) => void;
}) {
  const [author, setAuthor] = useState("");
  const [title, setTitle] = useState("");

  useEffect(() => {
    if (image) {
      setAuthor(image.author);
      setTitle(image.title);
    }
  }, [image]);

  return (
    <Dialog open={Boolean(image)} onClose={working ? undefined : onClose} maxWidth="xs" fullWidth>
      <DialogTitle sx={{ fontFamily: "var(--ad-font-display)", fontWeight: 650 }}>Edit image</DialogTitle>
      <DialogContent>
        {image && (
          <Box
            component="img"
            src={imageVariant(image.url, { w: 900, quality: 82 })}
            alt=""
            sx={{ width: "100%", borderRadius: 2, mb: 2 }}
          />
        )}
        <TextField
          label="Author"
          value={author}
          onChange={(event) => setAuthor(event.target.value)}
          fullWidth
          size="small"
          sx={{ mb: 1.5 }}
        />
        <TextField
          label="Title"
          value={title}
          onChange={(event) => setTitle(event.target.value)}
          fullWidth
          size="small"
        />
      </DialogContent>
      <DialogActions sx={{ px: 3, pb: 2.5 }}>
        <Button onClick={onClose} disabled={working} color="inherit">
          Cancel
        </Button>
        <Button
          onClick={() => onSave({ author: author.trim(), title })}
          disabled={working || !author.trim()}
          variant="contained"
        >
          {working ? "Saving…" : "Save"}
        </Button>
      </DialogActions>
    </Dialog>
  );
}

function UserEditDialog({
  user,
  working,
  onClose,
  onSave,
}: {
  user: User | null;
  working: boolean;
  onClose: () => void;
  onSave: (
    target: User,
    values: { displayName?: string; password?: string; isAdmin?: boolean },
  ) => void;
}) {
  const [displayName, setDisplayName] = useState("");
  const [password, setPassword] = useState("");
  const [isAdmin, setIsAdmin] = useState(false);

  useEffect(() => {
    if (user) {
      setDisplayName(user.displayName);
      setPassword("");
      setIsAdmin(user.isAdmin);
    }
  }, [user]);

  if (!user) return null;

  return (
    <Dialog open={Boolean(user)} onClose={working ? undefined : onClose} maxWidth="xs" fullWidth>
      <DialogTitle sx={{ fontFamily: "var(--ad-font-display)", fontWeight: 650 }}>
        @{user.username}
      </DialogTitle>
      <DialogContent>
        <Box sx={{ display: "grid", gap: 1.6, mt: 0.5 }}>
          <TextField
            label="Display name"
            value={displayName}
            onChange={(event) => setDisplayName(event.target.value)}
            size="small"
            fullWidth
          />
          <TextField
            label="New password (optional)"
            value={password}
            onChange={(event) => setPassword(event.target.value)}
            size="small"
            fullWidth
            type="password"
            placeholder="Leave empty to keep"
            inputProps={{ autoComplete: "new-password" }}
          />
          <FormControlLabel
            control={<Switch checked={isAdmin} onChange={(event) => setIsAdmin(event.target.checked)} />}
            label="Administrator"
          />
        </Box>
      </DialogContent>
      <DialogActions sx={{ px: 3, pb: 2.5 }}>
        <Button onClick={onClose} disabled={working} color="inherit">
          Cancel
        </Button>
        <Button
          onClick={() =>
            onSave(user, {
              displayName: displayName.trim() || user.username,
              password: password.trim() ? password : undefined,
              isAdmin,
            })
          }
          disabled={working}
          variant="contained"
        >
          {working ? "Saving…" : "Save"}
        </Button>
      </DialogActions>
    </Dialog>
  );
}

function CreateUserDialog({
  open,
  working,
  onClose,
  onSave,
}: {
  open: boolean;
  working: boolean;
  onClose: () => void;
  onSave: (values: { username: string; password: string; displayName: string; isAdmin: boolean }) => void;
}) {
  const [username, setUsername] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [password, setPassword] = useState("");
  const [isAdmin, setIsAdmin] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    if (open) {
      setUsername("");
      setDisplayName("");
      setPassword("");
      setIsAdmin(false);
      setError("");
    }
  }, [open]);

  const submit = () => {
    if (!/^[a-zA-Z0-9][a-zA-Z0-9_.-]{2,31}$/.test(username)) {
      setError("3–32 characters, letters, digits, _ . -");
      return;
    }
    if (password.length < 8) {
      setError("Password needs at least 8 characters.");
      return;
    }
    setError("");
    onSave({ username: username.trim(), password, displayName: displayName.trim(), isAdmin });
  };

  return (
    <Dialog open={open} onClose={working ? undefined : onClose} maxWidth="xs" fullWidth>
      <DialogTitle sx={{ fontFamily: "var(--ad-font-display)", fontWeight: 650 }}>New dreamer</DialogTitle>
      <DialogContent>
        <Box sx={{ display: "grid", gap: 1.5, mt: 0.5 }}>
          <TextField
            label="Username"
            value={username}
            onChange={(event) => setUsername(event.target.value)}
            size="small"
            fullWidth
          />
          <TextField
            label="Display name (optional)"
            value={displayName}
            onChange={(event) => setDisplayName(event.target.value)}
            size="small"
            fullWidth
          />
          <TextField
            label="Password"
            type="password"
            value={password}
            onChange={(event) => setPassword(event.target.value)}
            size="small"
            fullWidth
            inputProps={{ autoComplete: "new-password" }}
          />
          <FormControlLabel
            control={<Switch checked={isAdmin} onChange={(event) => setIsAdmin(event.target.checked)} />}
            label="Grant administrator"
          />
          {error && <Alert severity="warning">{error}</Alert>}
        </Box>
      </DialogContent>
      <DialogActions sx={{ px: 3, pb: 2.5 }}>
        <Button onClick={onClose} disabled={working} color="inherit">
          Cancel
        </Button>
        <Button onClick={submit} disabled={working} variant="contained">
          {working ? "Creating…" : "Create"}
        </Button>
      </DialogActions>
    </Dialog>
  );
}

function ModesDialog({
  open,
  modes,
  draft,
  working,
  onClose,
  onDraft,
  onSave,
  onDelete,
}: {
  open: boolean;
  modes: VoteMode[];
  draft: VoteMode | null;
  working: boolean;
  onClose: () => void;
  onDraft: (mode: VoteMode | null) => void;
  onSave: (draft: {
    id?: number;
    name: string;
    description: string;
    minValue: number;
    maxValue: number;
    isBinary: boolean;
  }) => void;
  onDelete: (mode: VoteMode) => void;
}) {
  const blank: VoteMode = {
    id: 0,
    name: "",
    description: "",
    minValue: 1,
    maxValue: 5,
    isBinary: false,
    createdAt: "",
  };
  const active = draft ?? blank;

  return (
    <Dialog open={open} onClose={working ? undefined : onClose} maxWidth="sm" fullWidth>
      <DialogTitle
        sx={{
          fontFamily: "var(--ad-font-display)",
          fontWeight: 650,
          display: "flex",
          alignItems: "center",
          gap: 1,
        }}
      >
        Voting modes
        <Chip size="small" label={`${modes.length}`} sx={{ ml: 0.5 }} />
      </DialogTitle>
      <DialogContent>
        <Box sx={{ display: "grid", gap: 1, mb: 2 }}>
          {modes.map((mode) => (
            <Box
              key={mode.id}
              sx={{
                display: "flex",
                alignItems: "center",
                gap: 1,
                p: 1,
                borderRadius: 2,
                border: "1px solid rgba(255,255,255,.1)",
                background: draft?.id === mode.id ? "rgba(255,255,255,.08)" : "transparent",
              }}
            >
              <Box sx={{ flex: 1, minWidth: 0 }}>
                <Typography sx={{ fontWeight: 700 }}>{mode.name}</Typography>
                <Typography variant="caption" color="text.secondary" noWrap component="div">
                  {mode.isBinary ? "duel" : `${mode.minValue} – ${mode.maxValue}`} · {mode.description || "no description"}
                </Typography>
              </Box>
              <Button size="small" onClick={() => onDraft(mode)}>
                Edit
              </Button>
              <IconButton size="small" color="error" onClick={() => onDelete(mode)}>
                <DeleteOutlineRounded fontSize="small" />
              </IconButton>
            </Box>
          ))}
        </Box>

        <Divider sx={{ my: 2 }} />
        <Typography sx={{ fontWeight: 700, mb: 1 }}>
          {draft?.id ? `Edit “${draft.name}”` : "New mode"}
        </Typography>
        <Box sx={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 1.4 }}>
          <TextField
            label="Name"
            size="small"
            value={active.name}
            onChange={(event) =>
              onDraft({ ...(draft ?? blank), name: event.target.value.toLowerCase(), id: draft?.id ?? 0 })
            }
            sx={{ gridColumn: "span 2" }}
          />
          <TextField
            label="Description"
            size="small"
            value={active.description}
            onChange={(event) => onDraft({ ...(draft ?? blank), description: event.target.value, id: draft?.id ?? 0 })}
            sx={{ gridColumn: "span 2" }}
          />
          <TextField
            label="Min value"
            type="number"
            size="small"
            value={active.minValue}
            onChange={(event) =>
              onDraft({ ...(draft ?? blank), minValue: Number(event.target.value), id: draft?.id ?? 0 })
            }
          />
          <TextField
            label="Max value"
            type="number"
            size="small"
            value={active.maxValue}
            onChange={(event) =>
              onDraft({ ...(draft ?? blank), maxValue: Number(event.target.value), id: draft?.id ?? 0 })
            }
          />
          <FormControlLabel
            control={
              <Switch
                checked={active.isBinary}
                onChange={(event) =>
                  onDraft({ ...(draft ?? blank), isBinary: event.target.checked, id: draft?.id ?? 0 })
                }
              />
            }
            label="Binary winner/loser mode"
            sx={{ gridColumn: "span 2" }}
          />
        </Box>
        <Box sx={{ display: "flex", gap: 1, mt: 2, flexWrap: "wrap" }}>
          <Button
            variant="contained"
            disabled={working || !active.name.trim() || active.maxValue <= active.minValue}
            onClick={() =>
              onSave({
                id: draft?.id || undefined,
                name: active.name.trim(),
                description: active.description.trim(),
                minValue: active.minValue,
                maxValue: active.maxValue,
                isBinary: active.isBinary,
              })
            }
            sx={{ borderRadius: 99, textTransform: "none" }}
          >
            {working ? "Saving…" : draft?.id ? "Update mode" : "Create mode"}
          </Button>
          {draft && (
            <Button onClick={() => onDraft(null)} sx={{ textTransform: "none" }}>
              Clear
            </Button>
          )}
        </Box>
      </DialogContent>
      <DialogActions sx={{ px: 3, pb: 2.5 }}>
        <Button onClick={onClose} disabled={working} color="inherit">
          Done
        </Button>
      </DialogActions>
    </Dialog>
  );
}

