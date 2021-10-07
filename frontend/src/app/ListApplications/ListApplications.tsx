import React from 'react';
import {
  Button,
  ButtonVariant,
  Bullseye,
  Toolbar,
  ToolbarItem,
  ToolbarContent,
  ToolbarFilter,
  ToolbarToggleGroup,
  ToolbarGroup,
  Dropdown,
  DropdownItem,
  DropdownPosition,
  DropdownToggle,
  InputGroup,
  PageSection,
  TextInput,
  Title,
  Select,
  SelectOption,
  SelectVariant,
  SelectGroup,
  EmptyState,
  EmptyStateIcon,
  EmptyStateBody,
  EmptyStateSecondaryActions
} from '@patternfly/react-core';
import { SearchIcon, FilterIcon } from '@patternfly/react-icons';
import { Table, TableHeader, TableBody} from '@patternfly/react-table';

import axios from 'axios';

const ListApplications: React.FunctionComponent = () => (
    <PageSection>
      <Title headingLevel="h1" size="lg">List Applications</Title>
  
      <SimpleForm />
  
    </PageSection>
  )
  
  
  export { ListApplications };
  
  async function doThing() {
  
    const url: string = 'your-url.example';
  
    try {
      const response = await axios.get("http://localhost:9000");
  
      console.log(`${response.status}`)
    } catch (exception) {
      console.error(`ERROR received from ${url}: ${exception}\n`);
    }
  }

class SimpleForm extends React.Component<{}, any> {
  constructor(props) {
    super(props);
    this.state = {
      filters: {
        DestinationURL: [],
        SourceURL: [],
        name: [],
        HealthStatus: [],
        SyncStatus :[]
      },
      currentCategory: 'Health Status',
      isFilterDropdownOpen: false,
      isCategoryDropdownOpen: false,
      nameInput: '',
      columns: [
        { title: 'Name' },
        { title: 'Source URL' },
        { title: 'Destination URL' },
        { title: 'Health Status' },
        { title: 'Sync Status' },
      ],
      rows: [
        { cells: ['Cluster 1', 'http://localhost:9001/', 'https://kubernetes.default.svc', 'Healthy', 'Synced'] },
        { cells: ['Cluster 2', 'http://localhost:9002/', 'https://kubernetes.default.svc','Progressing', 'Synced'] },
        { cells: ['Cluster 3',  'http://localhost:9003/', 'https://kubernetes.default.svc','Degraded', 'Synced']},
        { cells: ['Cluster 4', 'http://localhost:9004/', 'https://kubernetes.default.svc','Suspended', 'OutOfSync'] },
        { cells: ['Cluster 5', 'http://localhost:9005/', 'https://kubernetes.default.svc','Missing', 'OutOfSync'] },
        { cells: ['Cluster 6', 'http://localhost:9006/', 'https://kubernetes.default.svc','Unknown', 'OutOfSync'] }
      ],
      inputValue: ''
    };

    this.onDelete = (type = '', id = '') => {
      if (type) {
        this.setState(prevState => {
          prevState.filters[type.toLowerCase()] = prevState.filters[type.toLowerCase()].filter(s => s !== id);
          return {
            filters: prevState.filters
          };
        });
      } else {
        this.setState({
          filters: {
            DestinationURL: [],
            SourceURL: [],
            name: [],
            HealthStatus: [],
            SyncStatus: []
          }
        });
      }
    };

    this.onCategoryToggle = isOpen => {
      this.setState({
        isCategoryDropdownOpen: isOpen
      });
    };

    this.onCategorySelect = event => {
      this.setState({
        currentCategory: event.target.innerText,
        isCategoryDropdownOpen: !this.state.isCategoryDropdownOpen
      });
    };

    this.onFilterToggle = isOpen => {
      this.setState({
        isFilterDropdownOpen: isOpen
      });
    };

    this.onFilterSelect = event => {
      this.setState({
        isFilterDropdownOpen: !this.state.isFilterDropdownOpen
      });
    };

    this.onInputChange = newValue => {
      this.setState({ inputValue: newValue });
    };

    this.onRowSelect = (event, isSelected, rowId) => {
      let rows;
      if (rowId === -1) {
        rows = this.state.rows.map(oneRow => {
          oneRow.selected = isSelected;
          return oneRow;
        });
      } else {
        rows = [...this.state.rows];
        rows[rowId].selected = isSelected;
      }
      this.setState({
        rows
      });
    };

    this.onStatusSelect = (event, selection) => {
      const checked = event.target.checked;
      this.setState(prevState => {
        const prevSelections = prevState.filters['HealthStatus'];
        return {
          filters: {
            ...prevState.filters,
            HealthStatus: checked ? [...prevSelections, selection] : prevSelections.filter(value => value !== selection)
          }
        };
      });
    };

    this.onSyncStatusSelect = (event, selection) => {
      const checked = event.target.checked;
      this.setState(prevState => {
        const prevSelections = prevState.filters['SyncStatus'];
        return {
          filters: {
            ...prevState.filters,
            SyncStatus: checked ? [...prevSelections, selection] : prevSelections.filter(value => value !== selection)
          }
        };
      });
    };

    this.onNameInput = event => {
      if (event.key && event.key !== 'Enter') {
        return;
      }
      const { inputValue } = this.state;
      this.setState(prevState => {
        const prevFilters = prevState.filters['name'];
        return {
          filters: {
            ...prevState.filters,
            ['name']: prevFilters.includes(inputValue) ? prevFilters : [...prevFilters, inputValue]
          },
          inputValue: ''
        };
      });
    };

    this.onDestinationURLInput = event => {
      if (event.key && event.key !== 'Enter') {
        return;
      }
      const { inputValue } = this.state;
      this.setState(prevState => {
        const prevFilters = prevState.filters['DestinationURL'];
        return {
          filters: {
            ...prevState.filters,
            ['DestinationURL']: prevFilters.includes(inputValue) ? prevFilters : [...prevFilters, inputValue]
          },
          inputValue: ''
        };
      });
    };
 
    this.onSourceURLInput = event => {
      if (event.key && event.key !== 'Enter') {
        return;
      }
      const { inputValue } = this.state;
      this.setState(prevState => {
        const prevFilters = prevState.filters['SourceURL'];
        return {
          filters: {
            ...prevState.filters,
            ['SourceURL']: prevFilters.includes(inputValue) ? prevFilters : [...prevFilters, inputValue]
          },
          inputValue: ''
        };
      });
    };

  }
  buildCategoryDropdown() {
    const { isCategoryDropdownOpen, currentCategory } = this.state;

    return (
      <ToolbarItem>
        <Dropdown
          onSelect={this.onCategorySelect}
          position={DropdownPosition.left}
          toggle={
            <DropdownToggle onToggle={this.onCategoryToggle} style={{ width: '100%' }}>
              <FilterIcon /> {currentCategory}
            </DropdownToggle>
          }
          isOpen={isCategoryDropdownOpen}
          dropdownItems={[
            <DropdownItem key="cat1">Name</DropdownItem>,
            <DropdownItem key="cat2">Source URL</DropdownItem>,
            <DropdownItem key="cat3">Destination URL</DropdownItem>,
            <DropdownItem key="cat4">Health Status</DropdownItem>,
            <DropdownItem key="cat5">Sync Status</DropdownItem>
          ]}
          style={{ width: '100%' }}
        ></Dropdown>
      </ToolbarItem>
    );
  }

  buildFilterDropdown() {
    const { currentCategory, isFilterDropdownOpen, inputValue, filters } = this.state;

    const HealthStatusMenuItems = [ 
      <SelectOption key="statusHealthy" value="Healthy" />,
      <SelectOption key="statusProgressing" value="Progressing" />,
      <SelectOption key="statusMissing" value="Missing" />,
      <SelectOption key="statusSuspended" value="Suspended" />,
      <SelectOption key="statusDegraded" value="Degraded" />,
      <SelectOption key="statusUnknown" value="Unknown" />
    ];

    const SyncStatusMenuItems = [
      <SelectOption key="statusSynced" value="Synced" />,
      <SelectOption key="statusOutofSync" value="OutOfSync" />,
    ];

    return (
      <React.Fragment>

        <ToolbarFilter
          chips={filters.DestinationURL}
          deleteChip={this.onDelete}
          categoryName="Destination URL"
          showToolbarItem={currentCategory === 'Destination URL'}
        >
          <InputGroup>
            <TextInput
              name="URLInput"
              id="URLInput1"
              type="search"
              aria-label="URL filter"
              onChange={this.onInputChange}
              value={inputValue}
              placeholder="Filter by Destination URL..."
              onKeyDown={this.onDestinationURLInput}
            />
            <Button
              variant={ButtonVariant.control}
              aria-label="search button for search input"
              onClick={this.onDestinationURLInput}
            >
              <SearchIcon />
            </Button>
          </InputGroup>
        </ToolbarFilter>

        <ToolbarFilter
          chips={filters.SourceURL}
          deleteChip={this.onDelete}
          categoryName="Source URL"
          showToolbarItem={currentCategory === 'Source URL'}
        >
          <InputGroup>
            <TextInput
              name="URLInput"
              id="URLInput1"
              type="search"
              aria-label="URL filter"
              onChange={this.onInputChange}
              value={inputValue}
              placeholder="Filter by Source URL..."
              onKeyDown={this.onSourceURLInput}
            />
            <Button
              variant={ButtonVariant.control}
              aria-label="search button for search input"
              onClick={this.onSourceURLInput}
            >
              <SearchIcon />
            </Button>
          </InputGroup>
        </ToolbarFilter>


        <ToolbarFilter
          chips={filters.name}
          deleteChip={this.onDelete}
          categoryName="Name"
          showToolbarItem={currentCategory === 'Name'}
        >
          <InputGroup>
            <TextInput
              name="nameInput"
              id="nameInput1"
              type="search"
              aria-label="name filter"
              onChange={this.onInputChange}
              value={inputValue}
              placeholder="Filter by name..."
              onKeyDown={this.onNameInput}
            />
            <Button
              variant={ButtonVariant.control}
              aria-label="search button for search input"
              onClick={this.onNameInput}
            >
              <SearchIcon />
            </Button>
          </InputGroup>
        </ToolbarFilter>

        <ToolbarFilter
          chips={filters.HealthStatus}
          deleteChip={this.onDelete}
          categoryName="Health Status"
          showToolbarItem={currentCategory === 'Health Status'}
        >
          <Select
            variant={SelectVariant.checkbox}
            aria-label="Health Status"
            onToggle={this.onFilterToggle}
            onSelect={this.onStatusSelect}
            selections={filters.HealthStatus}
            isOpen={isFilterDropdownOpen}
            placeholderText="Filter by Health Status"
          >
            {HealthStatusMenuItems}
          </Select>
        </ToolbarFilter>

        <ToolbarFilter
          chips={filters.SyncStatus}
          deleteChip={this.onDelete}
          categoryName="Sync Status"
          showToolbarItem={currentCategory === 'Sync Status'}
        >
          <Select
            variant={SelectVariant.checkbox}
            aria-label="Sync Status"
            onToggle={this.onFilterToggle}
            onSelect={this.onSyncStatusSelect}
            selections={filters.SyncStatus}
            isOpen={isFilterDropdownOpen}
            placeholderText="Filter by Sync Status"
          >
            {SyncStatusMenuItems}
          </Select>
        </ToolbarFilter>

      </React.Fragment>
    );
  }

  renderToolbar() {
    const { filters } = this.state;
    return (
      <Toolbar
        id="toolbar-with-chip-groups"
        clearAllFilters={this.onDelete}
        collapseListedFiltersBreakpoint="xl"
      >
        <ToolbarContent>
          <ToolbarToggleGroup toggleIcon={<FilterIcon />} breakpoint="xl">
            <ToolbarGroup variant="filter-group">
              {this.buildCategoryDropdown()}
              {this.buildFilterDropdown()}
            </ToolbarGroup>
          </ToolbarToggleGroup>
        </ToolbarContent>
      </Toolbar>
    );
  }

  render() {
    const { loading, rows, columns, filters } = this.state;

    const filteredRows =
      filters.name.length > 0 || filters.SourceURL.length > 0 || filters.HealthStatus.length > 0 || filters.SyncStatus.length > 0 || filters.DestinationURL.length > 0 
        ? rows.filter(row => {
            return (
              (filters.name.length === 0 ||
                filters.name.some(name => row.cells[0].toLowerCase().includes(name.toLowerCase()))) &&
                (filters.SourceURL.length === 0 ||
                  filters.SourceURL.some(SourceURL => row.cells[1].toLowerCase().includes(SourceURL.toLowerCase())))  &&                
                (filters.DestinationURL.length === 0 ||
                    filters.DestinationURL.some(DestinationURL => row.cells[2].toLowerCase().includes(DestinationURL.toLowerCase())))  &&
              (filters.HealthStatus.length === 0 || filters.HealthStatus.includes(row.cells[3])) &&
              (filters.SyncStatus.length === 0 || filters.SyncStatus.includes(row.cells[4]))
            );
          })
        : rows;

    return (
      <React.Fragment>
        {this.renderToolbar()}
        {!loading && filteredRows.length > 0 && (
          <Table cells={columns} rows={filteredRows} onSelect={this.onRowSelect} aria-label="Filterable Table Demo">
            <TableHeader />
            <TableBody />
          </Table>
        )}
        {!loading && filteredRows.length === 0 && (
          <React.Fragment>
            <Table cells={columns} rows={filteredRows} onSelect={this.onRowSelect} aria-label="Filterable Table Demo">
              <TableHeader />
              <TableBody />
            </Table>
            <Bullseye>
              <EmptyState>
                <EmptyStateIcon icon={SearchIcon} />
                <Title headingLevel="h5" size="lg">
                  No results found
                </Title>
                <EmptyStateBody>
                  No results match this filter criteria. Remove all filters or clear all filters to show results.
                </EmptyStateBody>
                <EmptyStateSecondaryActions>
                  <Button variant="link" onClick={() => this.onDelete(null)}>
                    Clear all filters
                  </Button>
                </EmptyStateSecondaryActions>
              </EmptyState>
            </Bullseye>
          </React.Fragment>
        )}
        {loading && (
            <Title size="3xl" headingLevel={'h1'}>Please wait while loading data</Title>
        )}
      </React.Fragment>
    );
  }
}
