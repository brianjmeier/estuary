/**
 * TicketLinkModal Component
 * 
 * Modal for searching and linking Linear tickets to sessions.
 * Uses @inkjs/ui for TextInput and Select components.
 */

import { useState, useEffect, useCallback } from "react";
import { Box, Text, useInput } from "ink";
import { TextInput, Select } from "@inkjs/ui";
import Spinner from "ink-spinner";
import type { LinearTicket } from "../types.js";

interface TicketLinkModalProps {
  isOpen: boolean;
  onClose: () => void;
  onSelect: (ticket: LinearTicket) => void;
  searchTickets: (query: string) => Promise<LinearTicket[]>;
  isLinearInitialized: boolean;
}

export function TicketLinkModal({ 
  isOpen, 
  onClose, 
  onSelect,
  searchTickets,
  isLinearInitialized,
}: TicketLinkModalProps) {
  const [query, setQuery] = useState("");
  const [results, setResults] = useState<LinearTicket[]>([]);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [mode, setMode] = useState<"search" | "select">("search");

  // Reset state when modal opens/closes
  useEffect(() => {
    if (!isOpen) {
      setQuery("");
      setResults([]);
      setError(null);
      setMode("search");
    }
  }, [isOpen]);

  // Handle escape to close
  useInput((_input, key) => {
    if (!isOpen) return;
    if (key.escape) {
      onClose();
    }
    // Tab to switch between search and select modes
    if (key.tab && results.length > 0) {
      setMode(mode === "search" ? "select" : "search");
    }
  });

  // Debounced search effect
  useEffect(() => {
    if (!isOpen || query.length < 2 || !isLinearInitialized) {
      setResults([]);
      return;
    }

    const timer = setTimeout(async () => {
      setIsLoading(true);
      setError(null);
      try {
        const tickets = await searchTickets(query);
        setResults(tickets);
        if (tickets.length > 0) {
          setMode("select");
        }
      } catch (err) {
        setError(err instanceof Error ? err.message : "Search failed");
        setResults([]);
      } finally {
        setIsLoading(false);
      }
    }, 300);

    return () => clearTimeout(timer);
  }, [query, isOpen, searchTickets, isLinearInitialized]);

  const handleSelect = useCallback((value: string) => {
    const ticket = results.find((t) => t.identifier === value);
    if (ticket) {
      onSelect(ticket);
    }
  }, [results, onSelect]);

  if (!isOpen) return null;

  // Show setup message if Linear not initialized
  if (!isLinearInitialized) {
    return (
      <Box
        flexDirection="column"
        borderStyle="round"
        borderColor="yellow"
        paddingX={1}
        paddingY={1}
      >
        <Text bold color="yellow">Linear Not Configured</Text>
        <Box marginTop={1} flexDirection="column">
          <Text color="gray">To link tickets, add your Linear API key:</Text>
          <Text color="white">1. Create key at: linear.app/settings/api</Text>
          <Text color="white">2. Add to ~/.config/agenator/config.yaml:</Text>
          <Box marginLeft={2}>
            <Text color="cyan">linear:</Text>
          </Box>
          <Box marginLeft={4}>
            <Text color="cyan">apiKey: lin_api_xxxxx</Text>
          </Box>
        </Box>
        <Box marginTop={1}>
          <Text color="gray">[Esc] Close</Text>
        </Box>
      </Box>
    );
  }

  return (
    <Box
      flexDirection="column"
      borderStyle="round"
      borderColor="cyan"
      paddingX={1}
      paddingY={1}
    >
      <Text bold color="cyan">Link Linear Ticket</Text>
      
      <Box marginTop={1}>
        <Text color="gray">Search: </Text>
        {mode === "search" ? (
          <TextInput
            placeholder="Type ticket ID or title..."
            defaultValue={query}
            onChange={setQuery}
            isDisabled={mode !== "search"}
          />
        ) : (
          <Text color="white">{query}</Text>
        )}
      </Box>

      {isLoading && (
        <Box marginTop={1}>
          <Text color="cyan">
            <Spinner type="dots" />
          </Text>
          <Text color="gray"> Searching...</Text>
        </Box>
      )}

      {error && (
        <Box marginTop={1}>
          <Text color="red">{error}</Text>
        </Box>
      )}

      {!isLoading && results.length > 0 && mode === "select" && (
        <Box marginTop={1} flexDirection="column">
          <Text color="gray" dimColor>Use arrow keys, Enter to select:</Text>
          <Select
            options={results.map((ticket) => ({
              label: `${ticket.identifier}: ${ticket.title.slice(0, 50)}${ticket.title.length > 50 ? '...' : ''}`,
              value: ticket.identifier,
            }))}
            onChange={handleSelect}
          />
        </Box>
      )}

      {!isLoading && query.length >= 2 && results.length === 0 && !error && (
        <Box marginTop={1}>
          <Text color="gray">No tickets found</Text>
        </Box>
      )}

      {query.length < 2 && query.length > 0 && (
        <Box marginTop={1}>
          <Text color="gray">Type at least 2 characters to search</Text>
        </Box>
      )}

      <Box marginTop={1}>
        <Text color="gray">[Esc] Cancel</Text>
        {results.length > 0 && (
          <Text color="gray"> | [Tab] Switch to {mode === "search" ? "results" : "search"}</Text>
        )}
      </Box>
    </Box>
  );
}
