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
  EmptyState,
  EmptyStateIcon,
  EmptyStateBody,
  EmptyStateSecondaryActions
} from '@patternfly/react-core';
import { SearchIcon, FilterIcon } from '@patternfly/react-icons';
import { Table, TableHeader, TableBody} from '@patternfly/react-table';

import axios from 'axios';

const ListManagedCluster: React.FunctionComponent = () => (
    <PageSection>
      <Title headingLevel="h1" size="lg">List Managed Cluster</Title>
  
      <SimpleForm />
  
    </PageSection>
  )
  
  
  export { ListManagedCluster };
  
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
        URL: [],
        name: [],
        status: []
      },
      currentCategory: 'Status',
      isFilterDropdownOpen: false,
      isCategoryDropdownOpen: false,
      nameInput: '',
      columns: [
        { title: 'Name' },
        { title: 'Type' },
        { title: 'URL' },
        { title: 'Status' },
      ],
      rows: [
        { cells: ['Cluster 1', '5', 'http://localhost:9001/', 'Stopped'] },
        { cells: ['Cluster 2', '5', 'http://localhost:9002/', 'Down'] },
        { cells: ['Cluster 3', '13', 'http://localhost:9003/', 'Degraded'] },
        { cells: ['Cluster 4', '2', 'http://localhost:9004/', 'Needs Maintainence'] },
        { cells: ['Cluster 5', '7', 'http://localhost:9005/', 'Running'] },
        { cells: ['Cluster 6', '5', 'http://localhost:9006/', 'Stopped'] }
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
            URL: [],
            name: [],
            status: []
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
        const prevSelections = prevState.filters['status'];
        return {
          filters: {
            ...prevState.filters,
            status: checked ? [...prevSelections, selection] : prevSelections.filter(value => value !== selection)
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

    this.onURLInput = event => {
      if (event.key && event.key !== 'Enter') {
        return;
      }
      const { inputValue } = this.state;
      this.setState(prevState => {
        const prevFilters = prevState.filters['URL'];
        return {
          filters: {
            ...prevState.filters,
            ['URL']: prevFilters.includes(inputValue) ? prevFilters : [...prevFilters, inputValue]
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
            <DropdownItem key="cat1">URL</DropdownItem>,
            <DropdownItem key="cat2">Name</DropdownItem>,
            <DropdownItem key="cat3">Status</DropdownItem>
          ]}
          style={{ width: '100%' }}
        ></Dropdown>
      </ToolbarItem>
    );
  }

  buildFilterDropdown() {
    const { currentCategory, isFilterDropdownOpen, inputValue, filters } = this.state;

    const statusMenuItems = [
      <SelectOption key="statusRunning" value="Running" />,
      <SelectOption key="statusStopped" value="Stopped" />,
      <SelectOption key="statusDown" value="Down" />,
      <SelectOption key="statusDegraded" value="Degraded" />,
      <SelectOption key="statusMaint" value="Needs Maintainence" />
    ];

    return (
      <React.Fragment>

        <ToolbarFilter
          chips={filters.URL}
          deleteChip={this.onDelete}
          categoryName="URL"
          showToolbarItem={currentCategory === 'URL'}
        >
          <InputGroup>
            <TextInput
              name="URLInput"
              id="URLInput1"
              type="search"
              aria-label="URL filter"
              onChange={this.onInputChange}
              value={inputValue}
              placeholder="Filter by URL..."
              onKeyDown={this.onURLInput}
            />
            <Button
              variant={ButtonVariant.control}
              aria-label="search button for search input"
              onClick={this.onURLInput}
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
          chips={filters.status}
          deleteChip={this.onDelete}
          categoryName="Status"
          showToolbarItem={currentCategory === 'Status'}
        >
          <Select
            variant={SelectVariant.checkbox}
            aria-label="Status"
            onToggle={this.onFilterToggle}
            onSelect={this.onStatusSelect}
            selections={filters.status}
            isOpen={isFilterDropdownOpen}
            placeholderText="Filter by status"
          >
            {statusMenuItems}
          </Select>
        </ToolbarFilter>
      </React.Fragment>
    );
  }

  renderToolbar() {
    const { filters } = this.state;
    return (
      <Toolbar
        id="data-toolbar-with-chip-groups"
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
      filters.name.length > 0 || filters.URL.length > 0 || filters.status.length > 0
        ? rows.filter(row => {
            return (
              (filters.name.length === 0 ||
                filters.name.some(name => row.cells[0].toLowerCase().includes(name.toLowerCase()))) &&
                (filters.URL.length === 0 ||
                  filters.URL.some(URL => row.cells[2].toLowerCase().includes(URL.toLowerCase())))  &&
              (filters.status.length === 0 || filters.status.includes(row.cells[3]))
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
